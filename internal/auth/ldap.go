package auth

import (
	"crypto/tls"
	"fmt"
	"log"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/ryan/ads-registry/internal/config"
)

type LDAPClient struct {
	config config.LDAPConfig
}

func NewLDAPClient(cfg config.LDAPConfig) *LDAPClient {
	return &LDAPClient{
		config: cfg,
	}
}

func (c *LDAPClient) AuthenticateAndFetch(username, password string) ([]string, error) {
	if !c.config.Enabled {
		return nil, fmt.Errorf("LDAP is not enabled")
	}

	var l *ldap.Conn
	var err error

	dialLDAP := func() (*ldap.Conn, error) {
		if c.config.UseSSL {
			tlsConfig := &tls.Config{InsecureSkipVerify: c.config.InsecureSkipVerify}
			return ldap.DialTLS("tcp", c.config.Server, tlsConfig)
		}
		return ldap.Dial("tcp", c.config.Server)
	}

	l, err = dialLDAP()
	if err != nil {
		// Retry once after a short delay
		time.Sleep(500 * time.Millisecond)
		l, err = dialLDAP()
		if err != nil {
			return nil, fmt.Errorf("LDAP connection failed: %w", err)
		}
	}
	defer l.Close()

	// Set a connection-level timeout for all operations
	l.SetTimeout(10 * time.Second)

	// Initial bind with the service account
	err = l.Bind(c.config.BindDN, c.config.BindPassword)
	if err != nil {
		return nil, fmt.Errorf("LDAP admin bind failed: %w", err)
	}

	// Search for the user
	searchReq := ldap.NewSearchRequest(
		c.config.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf(c.config.UserSearchFilter, ldap.EscapeFilter(username)),
		[]string{"dn"},
		nil,
	)

	sr, err := l.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("LDAP user search failed: %w", err)
	}

	if len(sr.Entries) != 1 {
		return nil, fmt.Errorf("LDAP user not found or multiple entries found")
	}

	userDN := sr.Entries[0].DN

	// Bind as the user to verify password
	err = l.Bind(userDN, password)
	if err != nil {
		return nil, fmt.Errorf("LDAP user authentication failed: %w", err)
	}

	// Re-bind as admin to fetch groups
	err = l.Bind(c.config.BindDN, c.config.BindPassword)
	if err != nil {
		log.Printf("LDAP warning: could not re-bind as admin to fetch groups: %v", err)
	}

	// Fetch groups and map to scopes
	var scopes []string
	
	if c.config.GroupSearchFilter != "" {
		groupReq := ldap.NewSearchRequest(
			c.config.BaseDN,
			ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
			fmt.Sprintf(c.config.GroupSearchFilter, ldap.EscapeFilter(username)),
			[]string{"cn"},
			nil,
		)

		gr, err := l.Search(groupReq)
		if err != nil {
			log.Printf("LDAP warning: group search failed: %v", err)
		} else {
			for _, entry := range gr.Entries {
				cn := entry.GetAttributeValue("cn")
				if cn != "" {
					// Map this group to scopes if mapping exists
					if mappedScopes, ok := c.config.GroupToScopeMapping[cn]; ok {
						scopes = append(scopes, mappedScopes...)
					}
				}
			}
		}
	}

	return scopes, nil
}
