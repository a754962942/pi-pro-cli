package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
	"github.com/a754962942/pi-pro-cli/internal/assets"
	"github.com/a754962942/pi-pro-cli/internal/auth"
	"github.com/a754962942/pi-pro-cli/internal/client"
	"github.com/a754962942/pi-pro-cli/internal/config"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
	"github.com/a754962942/pi-pro-cli/internal/generation"
	"github.com/a754962942/pi-pro-cli/internal/lifecycle"
	"github.com/a754962942/pi-pro-cli/internal/output"
	"github.com/a754962942/pi-pro-cli/internal/schema"
	"github.com/a754962942/pi-pro-cli/internal/task"
)

type Options struct {
	ConfigDir       string
	ServerURL       string
	LocalVersion    string
	HTTPClient      lifecycle.HTTPClient
	ExecutablePath  string
	OperatingSystem string
	AuthPrompter    auth.Prompter
	TaskSleep       func(delay time.Duration)
	TaskJitter      task.JitterFunc
	CommandStdin    io.Reader
}

// Execute builds a fresh Cobra command tree for each invocation so tests and
// agent calls do not share flag state.
func Execute(args []string, stdout io.Writer, stderr io.Writer) int {
	return ExecuteWithOptions(args, stdout, stderr, Options{})
}

func ExecuteWithOptions(args []string, stdout io.Writer, stderr io.Writer, options Options) int {
	root := newRootCommand(stdout, stderr, options)
	root.SetArgs(args)

	if err := root.Execute(); err != nil {
		var appErr apperror.AppError
		if errors.As(err, &appErr) {
			return output.WriteError(stdout, appErr)
		}
		if strings.HasPrefix(err.Error(), "unknown command ") {
			return output.WriteError(stdout, apperror.Usage(errdefs.CodeUnknownCommand, err.Error()))
		}
		return output.WriteError(stdout, apperror.Usage(errdefs.CodeUsage, err.Error()))
	}

	return 0
}

func newRootCommand(stdout io.Writer, stderr io.Writer, options Options) *cobra.Command {
	var showVersion bool

	root := &cobra.Command{
		Use:           "pi-pro",
		Short:         "PI-Pro CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if showVersion {
				return writeVersion(stdout)
			}
			return apperror.Usage(errdefs.CodeUsage, errdefs.MessageMissingCommand)
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.CompletionOptions.DisableDefaultCmd = true
	root.Flags().BoolVarP(&showVersion, "version", "v", false, "Print version information")
	root.PersistentFlags().StringVar(&options.ServerURL, "server-url", options.ServerURL, "PI-Pro server URL")

	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_, _ = fmt.Fprintln(stdout, helpText())
	})

	root.AddCommand(newVersionCommand(stdout))
	root.AddCommand(newInitCommand(stdout, options))
	root.AddCommand(newUpdateCommand(stdout, options))
	root.AddCommand(newAuthCommand(stdout, stderr, options))
	root.AddCommand(newTypesCommand(stdout, options))
	root.AddCommand(newSchemaCommand(stdout, options))
	root.AddCommand(newTaskCommand(stdout, stderr, options))
	root.AddCommand(newGenerateCommand("generateImage", "image", stdout, stderr, options))
	root.AddCommand(newGenerateCommand("generateVideo", "video", stdout, stderr, options))

	return root
}

func lifecycleOptions(options Options) lifecycle.Options {
	return lifecycle.Options{
		ConfigDir:       options.ConfigDir,
		ServerURL:       options.ServerURL,
		LocalVersion:    options.LocalVersion,
		HTTPClient:      options.HTTPClient,
		ExecutablePath:  options.ExecutablePath,
		OperatingSystem: options.OperatingSystem,
	}
}

func authOptions(options Options, stderr io.Writer) auth.Options {
	prompter := options.AuthPrompter
	if prompter == nil {
		prompter = auth.NewTerminalPrompter(os.Stdin, stderr)
	}
	return auth.Options{
		ConfigDir:  options.ConfigDir,
		ServerURL:  options.ServerURL,
		HTTPClient: options.HTTPClient,
		Prompter:   prompter,
	}
}

func schemaRegistry(options Options) schema.Registry {
	configDir := options.ConfigDir
	if configDir == "" {
		if resolved, err := config.ResolveConfigDir(); err == nil {
			configDir = resolved
		}
	}
	paths := config.PathsFor(configDir)
	local := schema.NewLocalRegistry(paths.SchemasDir)
	serverURL := options.ServerURL
	if serverURL == "" {
		serverURL = config.Runtime().ServerURL
	}
	return schema.NewRemoteRegistry(context.Background(), client.New(client.Config{
		ServerURL:  serverURL,
		HTTPClient: options.HTTPClient,
	}), local)
}

func newInitCommand(stdout io.Writer, options Options) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize PI-Pro CLI runtime files",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := lifecycle.Init(cmd.Context(), lifecycleOptions(options))
			if err != nil {
				return err
			}
			if exitCode := output.WriteJSON(stdout, result); exitCode != 0 {
				return apperror.AppError{Code: errdefs.CodeOutputWriteFailed, Message: errdefs.MessageOutputWriteVersionFailed, Kind: apperror.KindIO}
			}
			return nil
		},
	}
}

func newUpdateCommand(stdout io.Writer, options Options) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update PI-Pro CLI binary",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := lifecycle.Update(cmd.Context(), lifecycleOptions(options))
			if err != nil {
				return err
			}
			if exitCode := output.WriteJSON(stdout, result); exitCode != 0 {
				return apperror.AppError{Code: errdefs.CodeOutputWriteFailed, Message: errdefs.MessageOutputWriteVersionFailed, Kind: apperror.KindIO}
			}
			return nil
		},
	}
}

func newVersionCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			return writeVersion(stdout)
		},
	}
}

func writeVersion(stdout io.Writer) error {
	exitCode := output.WriteJSON(stdout, map[string]any{
		"ok":           true,
		"localVersion": config.LocalVersion,
	})
	if exitCode != 0 {
		return apperror.AppError{Code: errdefs.CodeOutputWriteFailed, Message: errdefs.MessageOutputWriteVersionFailed, Kind: apperror.KindIO}
	}
	return nil
}

func newAuthCommand(stdout io.Writer, stderr io.Writer, options Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage PI-Pro authentication",
		RunE: func(cmd *cobra.Command, args []string) error {
			return apperror.Usage(errdefs.CodeUsage, errdefs.MessageAuthNotImplemented)
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "login",
		Short: "Log in to PI-Pro",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := auth.Login(cmd.Context(), authOptions(options, stderr))
			if err != nil {
				return err
			}
			if exitCode := output.WriteJSON(stdout, result); exitCode != 0 {
				return apperror.AppError{Code: errdefs.CodeOutputWriteFailed, Message: errdefs.MessageOutputWriteVersionFailed, Kind: apperror.KindIO}
			}
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "logout",
		Short: "Log out from PI-Pro",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := auth.Logout(authOptions(options, stderr))
			if err != nil {
				return err
			}
			if exitCode := output.WriteJSON(stdout, result); exitCode != 0 {
				return apperror.AppError{Code: errdefs.CodeOutputWriteFailed, Message: errdefs.MessageOutputWriteVersionFailed, Kind: apperror.KindIO}
			}
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show PI-Pro auth status",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := auth.Status(authOptions(options, stderr))
			if err != nil {
				return err
			}
			if exitCode := output.WriteJSON(stdout, result); exitCode != 0 {
				return apperror.AppError{Code: errdefs.CodeOutputWriteFailed, Message: errdefs.MessageOutputWriteVersionFailed, Kind: apperror.KindIO}
			}
			return nil
		},
	})
	return cmd
}

func newTypesCommand(stdout io.Writer, options Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "types",
		Short: "Inspect available PI-Pro generation types",
		RunE: func(cmd *cobra.Command, args []string) error {
			return apperror.Usage(errdefs.CodeUsage, errdefs.MessageTypesNotImplemented)
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List available generation types",
		RunE: func(cmd *cobra.Command, args []string) error {
			if result, ok := capabilityTypes(cmd.Context(), options); ok {
				if exitCode := output.WriteJSON(stdout, result); exitCode != 0 {
					return apperror.AppError{Code: errdefs.CodeOutputWriteFailed, Message: errdefs.MessageOutputWriteVersionFailed, Kind: apperror.KindIO}
				}
				return nil
			}
			types, err := schemaRegistry(options).List()
			if err != nil {
				return err
			}
			if exitCode := output.WriteJSON(stdout, map[string]any{"ok": true, "types": types}); exitCode != 0 {
				return apperror.AppError{Code: errdefs.CodeOutputWriteFailed, Message: errdefs.MessageOutputWriteVersionFailed, Kind: apperror.KindIO}
			}
			return nil
		},
	})

	var provider string
	var model string
	var schemaType string
	inspect := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect a generation type schema",
		RunE: func(cmd *cobra.Command, args []string) error {
			if provider == "" && model == "" && schemaType != "" {
				result, err := capabilityModels(cmd.Context(), options, schemaType)
				if err != nil {
					return err
				}
				if exitCode := output.WriteJSON(stdout, result); exitCode != 0 {
					return apperror.AppError{Code: errdefs.CodeOutputWriteFailed, Message: errdefs.MessageOutputWriteVersionFailed, Kind: apperror.KindIO}
				}
				return nil
			}
			if provider == "" || model == "" || schemaType == "" {
				return apperror.Usage(errdefs.CodeUsage, "provider, model, and type are required")
			}
			selected, err := schemaRegistry(options).Get(provider, model, schemaType)
			if err != nil {
				return err
			}
			if exitCode := output.WriteJSON(stdout, map[string]any{"ok": true, "schema": selected}); exitCode != 0 {
				return apperror.AppError{Code: errdefs.CodeOutputWriteFailed, Message: errdefs.MessageOutputWriteVersionFailed, Kind: apperror.KindIO}
			}
			return nil
		},
	}
	inspect.Flags().StringVar(&provider, "provider", "", "Model provider")
	inspect.Flags().StringVar(&model, "model", "", "Model name")
	inspect.Flags().StringVar(&schemaType, "type", "", "Generation behavior type")
	cmd.AddCommand(inspect)
	return cmd
}

func newSchemaCommand(stdout io.Writer, options Options) *cobra.Command {
	var brief bool
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Inspect PI-Pro schema contracts",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !brief {
				return apperror.Usage(errdefs.CodeUsage, "missing schema subcommand")
			}
			result, err := schemaBrief(cmd.Context(), options)
			if err != nil {
				return err
			}
			return writeCommandJSON(stdout, result)
		},
	}
	cmd.Flags().BoolVar(&brief, "brief", false, "Show compact remote capability and schema index")

	var provider string
	var model string
	var schemaType string
	inspect := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect one provider/model/type schema",
		RunE: func(cmd *cobra.Command, args []string) error {
			if provider == "" || model == "" || schemaType == "" {
				return apperror.Usage(errdefs.CodeUsage, "provider, model, and type are required")
			}
			selected, err := schemaRegistry(options).Get(provider, model, schemaType)
			if err != nil {
				return err
			}
			return writeCommandJSON(stdout, map[string]any{"ok": true, "schema": selected})
		},
	}
	inspect.Flags().StringVar(&provider, "provider", "", "Model provider")
	inspect.Flags().StringVar(&model, "model", "", "Model name")
	inspect.Flags().StringVar(&schemaType, "type", "", "Generation behavior type")
	cmd.AddCommand(inspect)
	return cmd
}

type capabilityTypeOutput struct {
	Type         string `json:"type"`
	DisplayName  string `json:"displayName,omitempty"`
	ArtifactKind string `json:"artifactKind"`
}

type schemaBriefTypeOutput struct {
	Type         string                   `json:"type"`
	DisplayName  string                   `json:"displayName,omitempty"`
	ArtifactKind string                   `json:"artifactKind"`
	Models       []client.CapabilityModel `json:"models"`
}

func schemaBrief(ctx context.Context, options Options) (map[string]any, error) {
	service, ok := capabilityClient(options)
	if ok {
		response, err := service.CapabilityTypes(ctx)
		if err == nil {
			types := make([]schemaBriefTypeOutput, 0, len(response.Types))
			for _, eventType := range response.Types {
				models, err := service.CapabilityModels(ctx, eventType.Code)
				if err != nil {
					return nil, err
				}
				types = append(types, schemaBriefTypeOutput{
					Type:         eventType.Code,
					DisplayName:  eventType.Name,
					ArtifactKind: eventType.ArtifactKind,
					Models:       models.Models,
				})
			}
			return map[string]any{"ok": true, "schemaSource": "remote", "types": types}, nil
		}
	}
	summaries, err := schemaRegistry(options).List()
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "schemaSource": "local", "types": summaries}, nil
}

func capabilityTypes(ctx context.Context, options Options) (map[string]any, bool) {
	service, ok := capabilityClient(options)
	if !ok {
		return nil, false
	}
	response, err := service.CapabilityTypes(ctx)
	if err != nil {
		return nil, false
	}
	types := make([]capabilityTypeOutput, 0, len(response.Types))
	for _, eventType := range response.Types {
		types = append(types, capabilityTypeOutput{
			Type:         eventType.Code,
			DisplayName:  eventType.Name,
			ArtifactKind: eventType.ArtifactKind,
		})
	}
	return map[string]any{"ok": true, "types": types}, true
}

func capabilityModels(ctx context.Context, options Options, eventType string) (map[string]any, error) {
	service, ok := capabilityClient(options)
	if !ok {
		return nil, apperror.Usage(errdefs.CodeUsage, "provider and model are required when capability API is not configured")
	}
	response, err := service.CapabilityModels(ctx, eventType)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "type": eventType, "models": response.Models}, nil
}

func capabilityClient(options Options) (*client.Client, bool) {
	serverURL := options.ServerURL
	if serverURL == "" {
		serverURL = config.Runtime().ServerURL
	}
	if serverURL == "" {
		return nil, false
	}
	return client.New(client.Config{
		ServerURL:  serverURL,
		HTTPClient: options.HTTPClient,
	}), true
}

func taskService(options Options, stderr io.Writer) task.Service {
	configDir := options.ConfigDir
	if configDir == "" {
		if resolved, err := config.ResolveConfigDir(); err == nil {
			configDir = resolved
		}
	}
	cfg, _ := config.Load(config.PathsFor(configDir))
	serverURL := options.ServerURL
	if serverURL == "" {
		serverURL = config.Runtime().ServerURL
	}
	taskOptions := task.Options{
		ServerURL:  serverURL,
		AuthToken:  cfg.AuthToken,
		HTTPClient: options.HTTPClient,
		Stderr:     stderr,
		Jitter:     options.TaskJitter,
	}
	if options.TaskSleep != nil {
		taskOptions.Sleeper = func(ctx context.Context, delay time.Duration) error {
			options.TaskSleep(delay)
			return nil
		}
	}
	return task.NewService(taskOptions)
}

func newTaskCommand(stdout io.Writer, stderr io.Writer, options Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Manage PI-Pro long-running tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			return apperror.Usage(errdefs.CodeUsage, errdefs.MessageTaskNotImplemented)
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "status <jobId>",
		Short: "Show task status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := taskService(options, stderr).Status(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return writeCommandJSON(stdout, result)
		},
	})

	var timeoutSeconds int
	var pollIntervalSeconds int
	var pollMaxSeconds int
	var pollBackoff float64
	var noJitter bool
	wait := &cobra.Command{
		Use:   "wait <jobId>",
		Short: "Wait for task completion",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskOptions := options
			if noJitter {
				taskOptions.TaskJitter = func(delay time.Duration) time.Duration {
					return delay
				}
			}
			service := taskService(taskOptions, stderr)
			result, err := service.Wait(cmd.Context(), args[0], task.WaitOptions{
				Timeout:         time.Duration(timeoutSeconds) * time.Second,
				InitialInterval: time.Duration(pollIntervalSeconds) * time.Second,
				MaxInterval:     time.Duration(pollMaxSeconds) * time.Second,
				Backoff:         pollBackoff,
			})
			if err != nil {
				return err
			}
			return writeCommandJSON(stdout, result)
		},
	}
	wait.Flags().IntVar(&timeoutSeconds, "timeout", 1800, "Polling timeout in seconds")
	wait.Flags().IntVar(&pollIntervalSeconds, "poll-interval", 2, "Initial polling interval in seconds")
	wait.Flags().IntVar(&pollMaxSeconds, "poll-max", 30, "Maximum polling interval in seconds")
	wait.Flags().Float64Var(&pollBackoff, "poll-backoff", 1.5, "Polling backoff multiplier")
	wait.Flags().BoolVar(&noJitter, "no-jitter", false, "Disable polling jitter")
	cmd.AddCommand(wait)

	cmd.AddCommand(&cobra.Command{
		Use:   "cancel <jobId>",
		Short: "Cancel task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := taskService(options, stderr).Cancel(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return writeCommandJSON(stdout, result)
		},
	})
	return cmd
}

func writeCommandJSON(stdout io.Writer, value any) error {
	if exitCode := output.WriteJSON(stdout, value); exitCode != 0 {
		return apperror.AppError{Code: errdefs.CodeOutputWriteFailed, Message: errdefs.MessageOutputWriteVersionFailed, Kind: apperror.KindIO}
	}
	return nil
}

func newGenerateCommand(name string, artifactKind string, stdout io.Writer, stderr io.Writer, options Options) *cobra.Command {
	var provider string
	var model string
	var schemaType string
	var inputPath string
	var dryRun bool
	var noWait bool
	var outputPath string
	var outputDir string
	var timeoutSeconds int
	var pollIntervalSeconds int
	var pollMaxSeconds int
	var pollBackoff float64
	var noJitter bool

	cmd := &cobra.Command{
		Use:   name,
		Short: "Submit PI-Pro generation request",
		RunE: func(cmd *cobra.Command, args []string) error {
			if provider == "" || model == "" || schemaType == "" {
				return apperror.Usage(errdefs.CodeUsage, "provider, model, and type are required")
			}
			service, cleanup, err := generationService(options, stderr, noJitter)
			if err != nil {
				return err
			}
			defer cleanup()
			result, err := service.Run(cmd.Context(), generation.Request{
				Command:       name,
				ArtifactKind:  artifactKind,
				Provider:      provider,
				Model:         model,
				Type:          schemaType,
				InputPath:     inputPath,
				DryRun:        dryRun,
				Wait:          !noWait,
				WaitSpecified: true,
				Timeout:       time.Duration(timeoutSeconds) * time.Second,
				PollInterval:  time.Duration(pollIntervalSeconds) * time.Second,
				PollMax:       time.Duration(pollMaxSeconds) * time.Second,
				PollBackoff:   pollBackoff,
				OutputPath:    outputPath,
				OutputDir:     outputDir,
			})
			if err != nil {
				return err
			}
			return writeCommandJSON(stdout, result)
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", "Model provider")
	cmd.Flags().StringVar(&model, "model", "", "Model name")
	cmd.Flags().StringVar(&schemaType, "type", "", "Generation behavior type")
	cmd.Flags().StringVar(&inputPath, "input", "", "JSON input file path or - for stdin")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate and print normalized request without submitting")
	cmd.Flags().StringVar(&outputPath, "output", "", "Artifact output file path")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Artifact output directory")
	cmd.Flags().BoolVar(&noWait, "no-wait", false, "Return immediately after task submission")
	cmd.Flags().IntVar(&timeoutSeconds, "timeout", 1800, "Polling timeout in seconds")
	cmd.Flags().IntVar(&pollIntervalSeconds, "poll-interval", 2, "Initial polling interval in seconds")
	cmd.Flags().IntVar(&pollMaxSeconds, "poll-max", 30, "Maximum polling interval in seconds")
	cmd.Flags().Float64Var(&pollBackoff, "poll-backoff", 1.5, "Polling backoff multiplier")
	cmd.Flags().BoolVar(&noJitter, "no-jitter", false, "Disable polling jitter")
	return cmd
}

func generationService(options Options, stderr io.Writer, noJitter bool) (generation.Service, func(), error) {
	configDir := options.ConfigDir
	if configDir == "" {
		if resolved, err := config.ResolveConfigDir(); err == nil {
			configDir = resolved
		}
	}
	paths := config.PathsFor(configDir)
	cfg, err := config.Load(paths)
	if err != nil {
		return generation.Service{}, func() {}, err
	}
	store, err := assets.Open(paths.AssetDB)
	if err != nil {
		return generation.Service{}, func() {}, err
	}
	stdin := options.CommandStdin
	if stdin == nil {
		stdin = os.Stdin
	}
	jitter := options.TaskJitter
	if noJitter {
		jitter = func(delay time.Duration) time.Duration {
			return delay
		}
	}
	serverURL := options.ServerURL
	if serverURL == "" {
		serverURL = config.Runtime().ServerURL
	}
	return generation.NewService(generation.Options{
		Registry:     schemaRegistry(options),
		Capabilities: client.New(client.Config{ServerURL: serverURL, HTTPClient: options.HTTPClient}),
		AssetResolver: assets.NewResolver(store, assets.HTTPUploader{
			ServerURL:  serverURL,
			AuthToken:  cfg.AuthToken,
			HTTPClient: options.HTTPClient,
		}),
		AssetStore: store,
		ServerURL:  serverURL,
		AuthToken:  cfg.AuthToken,
		HTTPClient: options.HTTPClient,
		Stdin:      stdin,
		Stderr:     stderr,
		TaskSleep:  options.TaskSleep,
		TaskJitter: jitter,
	}), func() { _ = store.Close() }, nil
}

func addProviderModelTypeFlags(cmd *cobra.Command) {
	cmd.Flags().String("provider", "", "Model provider")
	cmd.Flags().String("model", "", "Model name")
	cmd.Flags().String("type", "", "Generation behavior type")
}

func notImplementedCommand(use string, message string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: message,
		RunE: func(cmd *cobra.Command, args []string) error {
			return apperror.NotImplemented(errdefs.CodeNotImplemented, message)
		},
	}
}

func helpText() string {
	return `pi-pro

Usage:
  pi-pro init
  pi-pro update
  pi-pro auth login
  pi-pro auth logout
  pi-pro auth status
  pi-pro types list
  pi-pro types inspect --provider <provider> --model <model> --type <type>
  pi-pro schema --brief
  pi-pro schema inspect --provider <provider> --model <model> --type <type>
  pi-pro task status <jobId>
  pi-pro task wait <jobId>
  pi-pro task cancel <jobId>
  pi-pro generateImage --provider <provider> --model <model> --type <type>
  pi-pro generateVideo --provider <provider> --model <model> --type <type>

Global:
  pi-pro --help
  pi-pro --version
  pi-pro --server-url <url> <command>`
}
