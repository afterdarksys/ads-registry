package formats

import (
	"log"

	"github.com/go-chi/chi/v5"
	"github.com/ryan/ads-registry/internal/api/formats/apt"
	"github.com/ryan/ads-registry/internal/api/formats/brew"
	"github.com/ryan/ads-registry/internal/api/formats/cocoapods"
	"github.com/ryan/ads-registry/internal/api/formats/composer"
	"github.com/ryan/ads-registry/internal/api/formats/golang"
	"github.com/ryan/ads-registry/internal/api/formats/helm"
	"github.com/ryan/ads-registry/internal/api/formats/npm"
	"github.com/ryan/ads-registry/internal/api/formats/pypi"
	"github.com/ryan/ads-registry/internal/auth"
	"github.com/ryan/ads-registry/internal/db"
	"github.com/ryan/ads-registry/internal/storage"
)

// Router is the multiplexer for all package manager formats
type Router struct {
	db      db.Store
	storage storage.Provider
	authMid *auth.Middleware
}

// NewRouter initializes the registry formats subsystem
func NewRouter(dbStore db.Store, storageProvider storage.Provider, tokenService *auth.TokenService, devMode bool) *Router {
	return &Router{
		db:      dbStore,
		storage: storageProvider,
		authMid: auth.NewMiddleware(tokenService, devMode),
	}
}

// Register assembles the sub-routers for each available package format.
func (r *Router) Register(mux chi.Router) {
	mux.Route("/repository", func(api chi.Router) {
		log.Println("Initializing multi-format artifact handlers...")
		
		// Secure entire namespace using global token registry
		api.Use(r.authMid.Protect)

		// NPM
		npmHandler := npm.NewHandler(r.db, r.storage)
		api.Mount("/npm", npmHandler.Router())

		// PyPI
		pypiHandler := pypi.NewHandler(r.db, r.storage)
		api.Mount("/pypi", pypiHandler.Router())

		// APT
		aptHandler := apt.NewHandler(r.db, r.storage)
		api.Mount("/apt", aptHandler.Router())

		// Go Modules
		goHandler := golang.NewHandler(r.db, r.storage)
		api.Mount("/go", goHandler.Router())

		// Helm
		helmHandler := helm.NewHandler(r.db, r.storage)
		api.Mount("/helm", helmHandler.Router())

		// Composer
		composerHandler := composer.NewHandler(r.db, r.storage)
		api.Mount("/composer", composerHandler.Router())

		// CocoaPods
		cocoapodsHandler := cocoapods.NewHandler(r.db, r.storage)
		api.Mount("/cocoapods", cocoapodsHandler.Router())

		// Homebrew
		brewHandler := brew.NewHandler(r.db, r.storage)
		api.Mount("/brew", brewHandler.Router())
	})
}
