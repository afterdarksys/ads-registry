package ownership

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	ErrNotAuthorized     = errors.New("not authorized")
	ErrNotFound          = errors.New("not found")
	ErrAlreadyExists     = errors.New("already exists")
	ErrInvalidPermission = errors.New("invalid permission")
)

// Permission levels
const (
	PermissionPull   = "pull"
	PermissionPush   = "push"
	PermissionDelete = "delete"
	PermissionAdmin  = "admin"
)

// Group roles
const (
	RoleOwner       = "owner"
	RoleAdmin       = "admin"
	RoleContributor = "contributor"
	RoleMember      = "member"
	RoleReader      = "reader"
)

// Service handles ownership and permissions
type Service struct {
	db *sql.DB
}

// NewService creates a new ownership service
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// ============================================================================
// OWNERSHIP MANAGEMENT
// ============================================================================

// SetRepositoryOwner sets the owner of a repository
func (s *Service) SetRepositoryOwner(ctx context.Context, repoID, userID int) error {
	query := `UPDATE repositories SET owner_id = $1, updated_at = NOW() WHERE id = $2`
	result, err := s.db.ExecContext(ctx, query, userID, repoID)
	if err != nil {
		return fmt.Errorf("failed to set repository owner: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

// GetRepositoryOwner returns the owner user ID of a repository
func (s *Service) GetRepositoryOwner(ctx context.Context, repoID int) (int, error) {
	var ownerID sql.NullInt64
	query := `SELECT owner_id FROM repositories WHERE id = $1`
	err := s.db.QueryRowContext(ctx, query, repoID).Scan(&ownerID)
	if err != nil {
		return 0, fmt.Errorf("failed to get repository owner: %w", err)
	}

	if !ownerID.Valid {
		return 0, ErrNotFound
	}

	return int(ownerID.Int64), nil
}

// IsRepositoryOwner checks if a user is the owner of a repository
func (s *Service) IsRepositoryOwner(ctx context.Context, repoID, userID int) (bool, error) {
	ownerID, err := s.GetRepositoryOwner(ctx, repoID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return ownerID == userID, nil
}

// ============================================================================
// PERMISSION CHECKS
// ============================================================================

// HasPermission checks if a user has a specific permission on a repository
func (s *Service) HasPermission(ctx context.Context, repoID, userID int, permission string) (bool, error) {
	// Check if user is owner (owners have all permissions)
	isOwner, err := s.IsRepositoryOwner(ctx, repoID, userID)
	if err != nil {
		return false, err
	}
	if isOwner {
		return true, nil
	}

	// Check direct user permissions
	hasDirectPerm, err := s.hasDirectPermission(ctx, repoID, userID, permission)
	if err != nil {
		return false, err
	}
	if hasDirectPerm {
		return true, nil
	}

	// Check group permissions
	hasGroupPerm, err := s.hasGroupPermission(ctx, repoID, userID, permission)
	if err != nil {
		return false, err
	}

	return hasGroupPerm, nil
}

func (s *Service) hasDirectPermission(ctx context.Context, repoID, userID int, permission string) (bool, error) {
	query := `
		SELECT COUNT(*) > 0
		FROM repository_permissions
		WHERE repository_id = $1 AND user_id = $2 AND permission = $3
	`

	var hasPermission bool
	err := s.db.QueryRowContext(ctx, query, repoID, userID, permission).Scan(&hasPermission)
	if err != nil {
		return false, fmt.Errorf("failed to check direct permission: %w", err)
	}

	return hasPermission, nil
}

func (s *Service) hasGroupPermission(ctx context.Context, repoID, userID int, permission string) (bool, error) {
	query := `
		SELECT COUNT(*) > 0
		FROM repository_permissions rp
		JOIN group_members gm ON rp.group_id = gm.group_id
		WHERE rp.repository_id = $1 AND gm.user_id = $2 AND rp.permission = $3
	`

	var hasPermission bool
	err := s.db.QueryRowContext(ctx, query, repoID, userID, permission).Scan(&hasPermission)
	if err != nil {
		return false, fmt.Errorf("failed to check group permission: %w", err)
	}

	return hasPermission, nil
}

// CanPull checks if user can pull from repository
func (s *Service) CanPull(ctx context.Context, repoID, userID int) (bool, error) {
	return s.HasPermission(ctx, repoID, userID, PermissionPull)
}

// CanPush checks if user can push to repository
func (s *Service) CanPush(ctx context.Context, repoID, userID int) (bool, error) {
	return s.HasPermission(ctx, repoID, userID, PermissionPush)
}

// CanDelete checks if user can delete from repository
func (s *Service) CanDelete(ctx context.Context, repoID, userID int) (bool, error) {
	return s.HasPermission(ctx, repoID, userID, PermissionDelete)
}

// CanAdmin checks if user can administer repository
func (s *Service) CanAdmin(ctx context.Context, repoID, userID int) (bool, error) {
	return s.HasPermission(ctx, repoID, userID, PermissionAdmin)
}

// ============================================================================
// PERMISSION GRANTING
// ============================================================================

// GrantUserPermission grants a permission to a user on a repository
func (s *Service) GrantUserPermission(ctx context.Context, repoID, userID, grantedBy int, permission string) error {
	// Verify granter has admin permission
	canAdmin, err := s.CanAdmin(ctx, repoID, grantedBy)
	if err != nil {
		return err
	}
	if !canAdmin {
		return ErrNotAuthorized
	}

	query := `
		INSERT INTO repository_permissions (repository_id, user_id, permission, granted_by_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT DO NOTHING
	`
	_, err = s.db.ExecContext(ctx, query, repoID, userID, permission, grantedBy)
	if err != nil {
		return fmt.Errorf("failed to grant user permission: %w", err)
	}

	return nil
}

// GrantGroupPermission grants a permission to a group on a repository
func (s *Service) GrantGroupPermission(ctx context.Context, repoID, groupID, grantedBy int, permission string) error {
	// Verify granter has admin permission
	canAdmin, err := s.CanAdmin(ctx, repoID, grantedBy)
	if err != nil {
		return err
	}
	if !canAdmin {
		return ErrNotAuthorized
	}

	query := `
		INSERT INTO repository_permissions (repository_id, group_id, permission, granted_by_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT DO NOTHING
	`
	_, err = s.db.ExecContext(ctx, query, repoID, groupID, permission, grantedBy)
	if err != nil {
		return fmt.Errorf("failed to grant group permission: %w", err)
	}

	return nil
}

// RevokeUserPermission removes a permission from a user
func (s *Service) RevokeUserPermission(ctx context.Context, repoID, userID, revokedBy int, permission string) error {
	// Verify revoker has admin permission
	canAdmin, err := s.CanAdmin(ctx, repoID, revokedBy)
	if err != nil {
		return err
	}
	if !canAdmin {
		return ErrNotAuthorized
	}

	query := `
		DELETE FROM repository_permissions
		WHERE repository_id = $1 AND user_id = $2 AND permission = $3
	`
	_, err = s.db.ExecContext(ctx, query, repoID, userID, permission)
	if err != nil {
		return fmt.Errorf("failed to revoke user permission: %w", err)
	}

	return nil
}

// RevokeGroupPermission removes a permission from a group
func (s *Service) RevokeGroupPermission(ctx context.Context, repoID, groupID, revokedBy int, permission string) error {
	// Verify revoker has admin permission
	canAdmin, err := s.CanAdmin(ctx, repoID, revokedBy)
	if err != nil {
		return err
	}
	if !canAdmin {
		return ErrNotAuthorized
	}

	query := `
		DELETE FROM repository_permissions
		WHERE repository_id = $1 AND group_id = $2 AND permission = $3
	`
	_, err = s.db.ExecContext(ctx, query, repoID, groupID, permission)
	if err != nil {
		return fmt.Errorf("failed to revoke group permission: %w", err)
	}

	return nil
}

// ============================================================================
// GROUP MANAGEMENT
// ============================================================================

// Group represents a user group
type Group struct {
	ID          int
	Name        string
	Description string
	LDAPDn      string
	ADSid       string
	ExternalID  string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CreatedByID int
}

// CreateGroup creates a new group
func (s *Service) CreateGroup(ctx context.Context, name, description string, createdBy int) (*Group, error) {
	query := `
		INSERT INTO groups (name, description, created_by_id)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at
	`

	group := &Group{
		Name:        name,
		Description: description,
		CreatedByID: createdBy,
	}

	err := s.db.QueryRowContext(ctx, query, name, description, createdBy).Scan(
		&group.ID,
		&group.CreatedAt,
		&group.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create group: %w", err)
	}

	return group, nil
}

// GetGroup retrieves a group by ID
func (s *Service) GetGroup(ctx context.Context, groupID int) (*Group, error) {
	query := `
		SELECT id, name, description, COALESCE(ldap_dn, ''), COALESCE(ad_sid, ''),
		       COALESCE(external_id, ''), created_at, updated_at, created_by_id
		FROM groups
		WHERE id = $1
	`

	group := &Group{}
	err := s.db.QueryRowContext(ctx, query, groupID).Scan(
		&group.ID,
		&group.Name,
		&group.Description,
		&group.LDAPDn,
		&group.ADSid,
		&group.ExternalID,
		&group.CreatedAt,
		&group.UpdatedAt,
		&group.CreatedByID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get group: %w", err)
	}

	return group, nil
}

// AddGroupMember adds a user to a group
func (s *Service) AddGroupMember(ctx context.Context, groupID, userID, addedBy int, role string) error {
	query := `
		INSERT INTO group_members (group_id, user_id, role, added_by_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (group_id, user_id) DO UPDATE SET role = $3
	`

	_, err := s.db.ExecContext(ctx, query, groupID, userID, role, addedBy)
	if err != nil {
		return fmt.Errorf("failed to add group member: %w", err)
	}

	return nil
}

// RemoveGroupMember removes a user from a group
func (s *Service) RemoveGroupMember(ctx context.Context, groupID, userID int) error {
	query := `DELETE FROM group_members WHERE group_id = $1 AND user_id = $2`
	_, err := s.db.ExecContext(ctx, query, groupID, userID)
	if err != nil {
		return fmt.Errorf("failed to remove group member: %w", err)
	}

	return nil
}

// IsGroupMember checks if a user is a member of a group
func (s *Service) IsGroupMember(ctx context.Context, groupID, userID int) (bool, error) {
	query := `SELECT COUNT(*) > 0 FROM group_members WHERE group_id = $1 AND user_id = $2`

	var isMember bool
	err := s.db.QueryRowContext(ctx, query, groupID, userID).Scan(&isMember)
	if err != nil {
		return false, fmt.Errorf("failed to check group membership: %w", err)
	}

	return isMember, nil
}

// GetUserGroups returns all groups a user belongs to
func (s *Service) GetUserGroups(ctx context.Context, userID int) ([]*Group, error) {
	query := `
		SELECT g.id, g.name, g.description, COALESCE(g.ldap_dn, ''), COALESCE(g.ad_sid, ''),
		       COALESCE(g.external_id, ''), g.created_at, g.updated_at, g.created_by_id
		FROM groups g
		JOIN group_members gm ON g.id = gm.group_id
		WHERE gm.user_id = $1
		ORDER BY g.name
	`

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user groups: %w", err)
	}
	defer rows.Close()

	var groups []*Group
	for rows.Next() {
		group := &Group{}
		err := rows.Scan(
			&group.ID,
			&group.Name,
			&group.Description,
			&group.LDAPDn,
			&group.ADSid,
			&group.ExternalID,
			&group.CreatedAt,
			&group.UpdatedAt,
			&group.CreatedByID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan group: %w", err)
		}
		groups = append(groups, group)
	}

	return groups, nil
}

// ============================================================================
// REPOSITORY LISTING BY OWNERSHIP
// ============================================================================

// ListUserRepositories returns repositories owned by or accessible to a user
func (s *Service) ListUserRepositories(ctx context.Context, userID int) ([]int, error) {
	query := `
		SELECT DISTINCT r.id
		FROM repositories r
		LEFT JOIN repository_permissions rp_user ON r.id = rp_user.repository_id AND rp_user.user_id = $1
		LEFT JOIN repository_permissions rp_group ON r.id = rp_group.repository_id
		LEFT JOIN group_members gm ON rp_group.group_id = gm.group_id AND gm.user_id = $1
		WHERE r.owner_id = $1
		   OR rp_user.user_id IS NOT NULL
		   OR gm.user_id IS NOT NULL
		   OR r.visibility = 'public'
		ORDER BY r.id
	`

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list user repositories: %w", err)
	}
	defer rows.Close()

	var repoIDs []int
	for rows.Next() {
		var repoID int
		if err := rows.Scan(&repoID); err != nil {
			return nil, fmt.Errorf("failed to scan repository ID: %w", err)
		}
		repoIDs = append(repoIDs, repoID)
	}

	return repoIDs, nil
}
