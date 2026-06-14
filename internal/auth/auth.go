package auth

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
	"github.com/a754962942/pi-pro-cli/internal/client"
	"github.com/a754962942/pi-pro-cli/internal/config"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
	"github.com/a754962942/pi-pro-cli/internal/serverapi"
)

type HTTPClient = client.HTTPClient

type Prompter interface {
	PromptUsername() (string, error)
	PromptPassword() (string, error)
}

type Options struct {
	ConfigDir  string
	ServerURL  string
	HTTPClient client.HTTPClient
	Prompter   Prompter
}

type LoginResult struct {
	OK            bool   `json:"ok"`
	Authenticated bool   `json:"authenticated"`
	Username      string `json:"username"`
}

type StatusResult struct {
	OK            bool           `json:"ok"`
	Authenticated bool           `json:"authenticated"`
	Username      string         `json:"username,omitempty"`
	Source        map[string]any `json:"source,omitempty"`
}

type LogoutResult struct {
	OK            bool `json:"ok"`
	Authenticated bool `json:"authenticated"`
}

type loginResponse struct {
	AuthToken string `json:"authToken"`
	User      struct {
		Username string `json:"username"`
	} `json:"user"`
}

type TerminalPrompter struct {
	in  *os.File
	out io.Writer
}

func NewTerminalPrompter(in *os.File, out io.Writer) TerminalPrompter {
	return TerminalPrompter{in: in, out: out}
}

func (p TerminalPrompter) PromptUsername() (string, error) {
	_, _ = fmt.Fprint(p.out, "Username: ")
	line, err := bufio.NewReader(p.in).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func (p TerminalPrompter) PromptPassword() (string, error) {
	_, _ = fmt.Fprint(p.out, "Password: ")
	password, err := term.ReadPassword(int(p.in.Fd()))
	_, _ = fmt.Fprintln(p.out)
	if err != nil {
		return "", err
	}
	return string(password), nil
}

func Login(ctx context.Context, options Options) (LoginResult, error) {
	options = withDefaults(options)
	username, err := options.Prompter.PromptUsername()
	if err != nil {
		return LoginResult{}, promptError(err)
	}
	password, err := options.Prompter.PromptPassword()
	if err != nil {
		return LoginResult{}, promptError(err)
	}
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return LoginResult{}, apperror.AppError{
			Code:    errdefs.CodeAuthRequired,
			Message: "Username and password are required.",
			Kind:    apperror.KindAuth,
		}
	}

	api := client.New(client.Config{ServerURL: options.ServerURL, HTTPClient: options.HTTPClient})
	var response loginResponse
	if err := api.DoJSON(ctx, http.MethodPost, serverapi.AuthLogin, map[string]string{
		"username": username,
		"password": password,
	}, &response); err != nil {
		return LoginResult{}, err
	}
	if response.AuthToken == "" {
		return LoginResult{}, apperror.AppError{
			Code:    errdefs.CodeServerResponseInvalid,
			Message: "login response missing authToken",
			Kind:    apperror.KindNetwork,
		}
	}
	if response.User.Username != "" {
		username = response.User.Username
	}

	paths := config.PathsFor(options.ConfigDir)
	if err := config.Save(paths, config.File{AuthToken: response.AuthToken, Username: username}); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{OK: true, Authenticated: true, Username: username}, nil
}

func Status(options Options) (StatusResult, error) {
	options = withDefaults(options)
	cfg, err := config.Load(config.PathsFor(options.ConfigDir))
	if err != nil {
		return StatusResult{}, err
	}
	if cfg.AuthToken == "" {
		return StatusResult{OK: true, Authenticated: false}, nil
	}
	return StatusResult{
		OK:            true,
		Authenticated: true,
		Username:      cfg.Username,
		Source: map[string]any{
			"authToken": "user-config",
		},
	}, nil
}

func Logout(options Options) (LogoutResult, error) {
	options = withDefaults(options)
	if err := config.Save(config.PathsFor(options.ConfigDir), config.File{}); err != nil {
		return LogoutResult{}, err
	}
	return LogoutResult{OK: true, Authenticated: false}, nil
}

func withDefaults(options Options) Options {
	if options.ConfigDir == "" {
		if configDir, err := config.ResolveConfigDir(); err == nil {
			options.ConfigDir = configDir
		}
	}
	if options.ServerURL == "" {
		options.ServerURL = config.BuiltInServerURL
	}
	if options.HTTPClient == nil {
		options.HTTPClient = http.DefaultClient
	}
	if options.Prompter == nil {
		options.Prompter = NewTerminalPrompter(os.Stdin, os.Stderr)
	}
	return options
}

func promptError(err error) error {
	return apperror.AppError{
		Code:    errdefs.CodeUsage,
		Message: "failed to read interactive auth input",
		Kind:    apperror.KindUsage,
		Details: map[string]any{
			"error": err.Error(),
		},
	}
}
