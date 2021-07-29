package create

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/redhat-developer/app-services-cli/pkg/cmdutil"
	"github.com/redhat-developer/app-services-cli/pkg/connection"
	"github.com/redhat-developer/app-services-cli/pkg/dump"
	"github.com/redhat-developer/app-services-cli/pkg/localize"
	registryinstanceclient "github.com/redhat-developer/app-services-sdk-go/registryinstance/apiv1internal/client"
	"gopkg.in/yaml.v2"

	"github.com/redhat-developer/app-services-cli/pkg/cmd/flag"
	"github.com/redhat-developer/app-services-cli/pkg/cmd/registry/artifacts/util"
	flagutil "github.com/redhat-developer/app-services-cli/pkg/cmdutil/flags"

	"github.com/redhat-developer/app-services-cli/pkg/iostreams"

	"github.com/redhat-developer/app-services-cli/pkg/logging"

	"github.com/spf13/cobra"

	"github.com/redhat-developer/app-services-cli/internal/config"
	"github.com/redhat-developer/app-services-cli/pkg/cmd/factory"
)

type Options struct {
	artifact string
	group    string

	file         string
	artifactType string
	version      string

	registryID   string
	outputFormat string

	IO         *iostreams.IOStreams
	Config     config.IConfig
	Connection factory.ConnectionFunc
	Logger     func() (logging.Logger, error)
	localizer  localize.Localizer
}

// NewCreateCommand creates a new command for creating registry.
func NewCreateCommand(f *factory.Factory) *cobra.Command {
	opts := &Options{
		IO:         f.IOStreams,
		Config:     f.Config,
		Connection: f.Connection,
		Logger:     f.Logger,
		localizer:  f.Localizer,
	}

	cmd := &cobra.Command{
		Use:     "create",
		Short:   "Creates new artifact",
		Long:    "Creates new artifact from file or directly from content.",
		Example: "",
		Args:    cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			validOutputFormats := flagutil.ValidOutputFormats
			if opts.outputFormat != "" && !flagutil.IsValidInput(opts.outputFormat, validOutputFormats...) {
				return flag.InvalidValueError("output", opts.outputFormat, validOutputFormats...)
			}

			if len(args) > 0 {
				opts.file = args[0]
			}

			if opts.registryID != "" {
				return runCreate(opts)
			}

			cfg, err := opts.Config.Load()
			if err != nil {
				return err
			}

			// TODO validate artifact types

			if !cfg.HasServiceRegistry() {
				return fmt.Errorf("No service Registry selected. Use rhoas registry use to select your registry")
			}

			opts.registryID = fmt.Sprint(cfg.Services.ServiceRegistry.InstanceID)
			return runCreate(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.outputFormat, "output", "o", "json", opts.localizer.MustLocalize("registry.cmd.flag.output.description"))
	cmd.Flags().StringVarP(&opts.file, "file", "f", "", "File location of the artifact")

	cmd.Flags().StringVarP(&opts.artifact, "artifact", "a", "", "Id of the artifact")
	cmd.Flags().StringVarP(&opts.group, "group", "g", "", "Id of the artifact")
	cmd.Flags().StringVarP(&opts.artifactType, "type", "t", "", "Type of artifact")
	cmd.Flags().StringVarP(&opts.version, "version", "", "", "Force specific version of the artifact")
	cmd.Flags().StringVarP(&opts.registryID, "registryId", "", "", "Id of the registry to be used. By default uses currently selected registry.")

	flagutil.EnableOutputFlagCompletion(cmd)

	return cmd
}

// nolint:funlen
func runCreate(opts *Options) error {

	logger, err := opts.Logger()
	if err != nil {
		return err
	}

	conn, err := opts.Connection(connection.DefaultConfigRequireMasAuth)
	if err != nil {
		return err
	}

	dataAPI, _, err := conn.API().ServiceRegistryInstance(opts.registryID)
	if err != nil {
		return err
	}

	if opts.group == "" {
		logger.Info("Group was not specified. Using 'default' artifacts group.")
		opts.group = util.DefaultArtifactGroup
	}

	// TODO read from STDIN when file is not provided
	if opts.file != "" {
		logger.Info("Opening file for reading")
		specifiedFile, err := os.Open(opts.file)
		if err != nil {
			return err
		}
		ctx := context.Background()
		request := dataAPI.ArtifactsApi.CreateArtifact(ctx, opts.group)
		if opts.artifactType != "" {
			request = request.XRegistryArtifactType(registryinstanceclient.ArtifactType(opts.artifactType))
		}
		if opts.artifact != "" {
			request = request.XRegistryArtifactId(opts.artifact)
		}
		if opts.version != "" {
			request = request.XRegistryVersion(opts.version)
		}
		request = request.Body(specifiedFile)
		metadata, _, err := request.Execute()
		if err != nil {
			return err
		}
		logger.Info("Artifact created")

		switch opts.outputFormat {
		case "json":
			data, _ := json.MarshalIndent(metadata, "", cmdutil.DefaultJSONIndent)
			_ = dump.JSON(opts.IO.Out, data)
		case "yaml", "yml":
			data, _ := yaml.Marshal(metadata)
			_ = dump.YAML(opts.IO.Out, data)
		}

	}

	return nil
}
