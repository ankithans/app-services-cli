package get

import (
	"context"
	"fmt"
	"os"

	flagutil "github.com/redhat-developer/app-services-cli/pkg/cmdutil/flags"
	"github.com/redhat-developer/app-services-cli/pkg/connection"
	"github.com/redhat-developer/app-services-cli/pkg/iostreams"
	"github.com/redhat-developer/app-services-cli/pkg/localize"
	"github.com/redhat-developer/app-services-cli/pkg/serviceregistry/registryinstanceerror"
	"github.com/spf13/cobra"

	"github.com/redhat-developer/app-services-cli/internal/config"
	"github.com/redhat-developer/app-services-cli/pkg/cmd/factory"
	"github.com/redhat-developer/app-services-cli/pkg/cmd/registry/artifacts/util"

	"github.com/redhat-developer/app-services-cli/pkg/logging"
)

type Options struct {
	artifact string
	group    string

	version string

	registryID string

	IO         *iostreams.IOStreams
	Config     config.IConfig
	Logger     func() (logging.Logger, error)
	Connection factory.ConnectionFunc
	localizer  localize.Localizer
}

// NewDescribeCommand describes a service instance, either by passing an `--id flag`
// or by using the service instance set in the config, if any
func NewGetCommand(f *factory.Factory) *cobra.Command {
	opts := &Options{
		Config:     f.Config,
		Connection: f.Connection,
		IO:         f.IOStreams,
		localizer:  f.Localizer,
		Logger:     f.Logger,
	}

	cmd := &cobra.Command{
		Use:     "get",
		Short:   "Get latest artifact by id and group",
		Long:    "",
		Example: "",
		Args:    cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.artifact = args[0]
			}

			if len(args) > 0 {
				opts.artifact = args[0]
			}

			if opts.registryID != "" {
				return runGet(opts)
			}

			cfg, err := opts.Config.Load()
			if err != nil {
				return err
			}

			if !cfg.HasServiceRegistry() {
				return fmt.Errorf("No service Registry selected. Use 'rhoas service-registry use' to select your registry")
			}

			opts.registryID = fmt.Sprint(cfg.Services.ServiceRegistry.InstanceID)
			return runGet(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.artifact, "artifact", "a", "", "Id of the artifact")
	cmd.Flags().StringVarP(&opts.group, "group", "g", "", "Id of the artifact")
	cmd.Flags().StringVarP(&opts.registryID, "registryId", "", "", "Id of the registry to be used. By default uses currently selected registry.")

	flagutil.EnableOutputFlagCompletion(cmd)

	return cmd
}

func runGet(opts *Options) error {
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

	logger.Info("Fetching artifact")

	if err != nil {
		return nil
	}

	ctx := context.Background()
	request := dataAPI.ArtifactsApi.GetLatestArtifact(ctx, opts.group, opts.artifact)
	data, _, err := request.Execute()
	if err != nil {
		return registryinstanceerror.TransformError(err)
	}

	fmt.Fprintf(os.Stdout, "%v\n", data)

	// stringContent := new(strings.Builder)
	// io.Copy(stringContent, data)
	// if err != nil {
	// 	return nil
	// }
	// io.WriteString(opts.IO.Out, stringContent.String())
	logger.Info("Successfully downloaded artifact")

	return nil
}
