package login

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/phayes/freeport"
	"github.com/redhat-developer/app-services-cli/internal/config"
	"github.com/redhat-developer/app-services-cli/pkg/auth/pkce"
	"github.com/redhat-developer/app-services-cli/pkg/browser"
	"github.com/redhat-developer/app-services-cli/pkg/iostreams"
	"github.com/redhat-developer/app-services-cli/pkg/localize"
	"github.com/redhat-developer/app-services-cli/pkg/logging"
	"github.com/redhat-developer/app-services-cli/static"
	"golang.org/x/oauth2"
)

type AuthorizationCodeGrant struct {
	HTTPClient *http.Client
	Config     config.IConfig
	Logger     logging.Logger
	IO         *iostreams.IOStreams
	Localizer  localize.Localizer
	ClientID   string
	Scopes     []string
	PrintURL   bool
}

type SSOConfig struct {
	AuthURL      string
	RedirectPath string
}

// Execute runs an Authorization Code flow login
// enabling the user to log in to SSO and MAS-SSO in succession
// https://tools.ietf.org/html/rfc6749#section-4.1
func (a *AuthorizationCodeGrant) Execute(ctx context.Context, ssoCfg *SSOConfig, masSSOCfg *SSOConfig) error {
	// log in to SSO
	a.Logger.Info(a.Localizer.MustLocalize("login.log.info.loggingIn"))
	if err := a.loginSSO(ctx, ssoCfg); err != nil {
		return err
	}
	a.Logger.Info(a.Localizer.MustLocalize("login.log.info.loggedIn"))

	a.Logger.Info(a.Localizer.MustLocalize("login.log.info.loggingInMAS"))
	// log in to MAS-SSO
	if err := a.loginMAS(ctx, masSSOCfg); err != nil {
		return err
	}
	a.Logger.Info(a.Localizer.MustLocalize("login.log.info.loggedInMAS"))

	return nil
}

// log the user in to the main authorization server
// this can be configured with the `--auth-url` flag
func (a *AuthorizationCodeGrant) loginSSO(ctx context.Context, cfg *SSOConfig) error {
	a.Logger.Debug("Logging into", cfg.AuthURL, "\n")
	clientCtx, cancel := createClientContext(ctx, a.HTTPClient)
	defer cancel()
	provider, err := oidc.NewProvider(ctx, cfg.AuthURL)
	if err != nil {
		return err
	}

	redirectURL, redirectURLPort, err := createRedirectURL(cfg.RedirectPath)
	if err != nil {
		return err
	}

	oauthConfig := &oauth2.Config{
		ClientID:    a.ClientID,
		Endpoint:    provider.Endpoint(),
		RedirectURL: redirectURL.String(),
		Scopes:      a.Scopes,
	}

	oidcConfig := &oidc.Config{
		ClientID: a.ClientID,
	}

	verifier := provider.Verifier(oidcConfig)
	state, _ := pkce.GenerateVerifier(128)

	// PKCE
	pkceCodeVerifier, err := pkce.GenerateVerifier(128)
	if err != nil {
		return err
	}
	pkceCodeChallenge := pkce.CreateChallenge(pkceCodeVerifier)
	authCodeURL := oauthConfig.AuthCodeURL(state, *pkce.GetAuthCodeURLOptions(pkceCodeChallenge)...)
	a.Logger.Debug("Opening Authorization URL:", authCodeURL)
	a.Logger.Info()

	// create a localhost server to handle redirects and exchange tokens securely
	sm := http.NewServeMux()
	server := http.Server{
		Handler: sm,
		Addr:    redirectURL.Host,
	}

	sm.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, authCodeURL, http.StatusFound)
	})

	sm.Handle("/static/", createStaticHTTPHandler())

	authURL, err := url.Parse(cfg.AuthURL)
	if err != nil {
		return err
	}

	// HTTP handler for the redirect URL
	sm.Handle("/"+redirectURL.Path, &redirectPageHandler{
		CancelContext: cancel,
		Ctx:           clientCtx,
		Port:          redirectURLPort,
		Config:        a.Config,
		Logger:        a.Logger,
		IO:            a.IO,
		ServerAddr:    server.Addr,
		Oauth2Config:  oauthConfig,
		State:         state,
		TokenVerifier: verifier,
		AuthURL:       authURL,
		Localizer:     a.Localizer,
		AuthOptions: []oauth2.AuthCodeOption{
			oauth2.SetAuthURLParam("code_verifier", pkceCodeVerifier),
			oauth2.SetAuthURLParam("grant_type", "authorization_code"),
		},
	})

	a.openBrowser(authCodeURL, redirectURL)

	// start the local server
	a.startServer(clientCtx, &server)

	return nil
}

// log in to MAS-SSO
func (a *AuthorizationCodeGrant) loginMAS(ctx context.Context, cfg *SSOConfig) error {
	a.Logger.Debug("Logging into", cfg.AuthURL, "\n")

	clientCtx, cancel := createClientContext(ctx, a.HTTPClient)
	defer cancel()
	provider, err := oidc.NewProvider(ctx, cfg.AuthURL)
	if err != nil {
		return err
	}

	redirectURL, redirectURLPort, err := createRedirectURL(cfg.RedirectPath)
	if err != nil {
		return err
	}

	oauthConfig := &oauth2.Config{
		ClientID:    a.ClientID,
		Endpoint:    provider.Endpoint(),
		RedirectURL: redirectURL.String(),
		Scopes:      a.Scopes,
	}

	oidcConfig := &oidc.Config{
		ClientID: a.ClientID,
	}

	// Configure PKCE challenge and verifier
	// https://tools.ietf.org/html/rfc7636
	verifier := provider.Verifier(oidcConfig)
	state, _ := pkce.GenerateVerifier(128)
	pkceCodeVerifier, err := pkce.GenerateVerifier(128)
	if err != nil {
		return err
	}
	pkceCodeChallenge := pkce.CreateChallenge(pkceCodeVerifier)

	authCodeURL := oauthConfig.AuthCodeURL(state, *pkce.GetAuthCodeURLOptions(pkceCodeChallenge)...)
	a.Logger.Debug("Opening Authorization URL:", authCodeURL)
	a.Logger.Info()

	sm := http.NewServeMux()
	server := http.Server{
		Handler: sm,
		Addr:    redirectURL.Host,
	}

	sm.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, authCodeURL, http.StatusFound)
	})

	sm.Handle("/static/", createStaticHTTPHandler())

	// HTTP handler for the redirect page
	sm.Handle("/"+redirectURL.Path, &masRedirectPageHandler{
		CancelContext: cancel,
		Ctx:           clientCtx,
		Port:          redirectURLPort,
		Config:        a.Config,
		Logger:        a.Logger,
		IO:            a.IO,
		ServerAddr:    server.Addr,
		Oauth2Config:  oauthConfig,
		State:         state,
		TokenVerifier: verifier,
		Localizer:     a.Localizer,
		AuthOptions: []oauth2.AuthCodeOption{
			oauth2.SetAuthURLParam("code_verifier", pkceCodeVerifier),
			oauth2.SetAuthURLParam("grant_type", "authorization_code"),
		},
	})

	a.openBrowser(authCodeURL, redirectURL)
	a.startServer(clientCtx, &server)

	return nil
}

func (a *AuthorizationCodeGrant) openBrowser(authCodeURL string, redirectURL *url.URL) {
	if a.PrintURL {
		a.Logger.Info(a.Localizer.MustLocalize("login.log.info.openSSOUrl"), "\n")
		fmt.Fprintln(a.IO.Out, authCodeURL)
		a.Logger.Info("")
	} else {
		err := browser.Open(redirectURL.Scheme + "://" + redirectURL.Host)
		if err != nil {
			a.printAuthURLFallback(authCodeURL, redirectURL, err)
			return
		}
	}
}

// starts the local HTTP webserver to handle redirect from the Auth server
func (a *AuthorizationCodeGrant) startServer(ctx context.Context, server *http.Server) {
	go func() {
		log.Fatal(server.ListenAndServe())
	}()
	<-ctx.Done()
}

// create an OIDC client context which is cancellable
func createClientContext(ctx context.Context, httpClient *http.Client) (context.Context, context.CancelFunc) {
	cancelCtx, cancel := context.WithCancel(ctx)
	clientCtx := oidc.ClientContext(cancelCtx, httpClient)

	return clientCtx, cancel
}

// creates a redirect URL with a random port which is available
// on the user's system
func createRedirectURL(path string) (*url.URL, int, error) {
	port, err := freeport.GetFreePort()
	if err != nil {
		return nil, 0, err
	}

	redirectURL := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("localhost:%v", port),
		Path:   path,
	}

	return redirectURL, port, nil
}

// when there is an error trying to automatically open the browser on the login page
// fallback to printing the URL to the user terminal instead.
func (a *AuthorizationCodeGrant) printAuthURLFallback(authCodeURL string, redirectURL *url.URL, err error) {
	a.PrintURL = true
	a.Logger.Debug("Error opening browser:", err, "\nPrinting Auth URL to console instead")
	a.openBrowser(authCodeURL, redirectURL)
}

func createStaticHTTPHandler() http.Handler {
	staticFs := http.FileServer(http.FS(static.ImagesFS()))
	return http.StripPrefix("/static", staticFs)
}
