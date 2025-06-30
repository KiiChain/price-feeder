package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"cosmossdk.io/math"

	input "github.com/cosmos/cosmos-sdk/client/input"
	"github.com/cosmos/cosmos-sdk/telemetry"

	"github.com/kiichain/price-feeder/config"
	"github.com/kiichain/price-feeder/oracle"
	"github.com/kiichain/price-feeder/oracle/client"
	v1 "github.com/kiichain/price-feeder/router/v1"
)

var (
	// Define the keyring password
	envVariablePass = "PRICE_FEEDER_PASS"

	// Define different flags for the command
	FlagSkipPassword = "skip-password"
)

var startCMD = &cobra.Command{
	Use:   "start [config-file]",
	Args:  cobra.ExactArgs(1),
	Short: "starts the price-feeder process with a given configuration file",
	Long: `starts the price-feeder process with a given configuration file.
The environment variable PRICE_FEEDER_PASS can be used to set the keyring password.
If the flag --skip-password is set, the keyring password prompt will be skipped.`,
	RunE: priceFeederCmdHandler,
}

func init() {
	// set the start command's flags
	startCMD.Flags().Bool(FlagSkipPassword, false, "skip keyring password prompt. Useful if using keyring test.")
}

// priceFeederCmdHandler init the price feeder
func priceFeederCmdHandler(cmd *cobra.Command, args []string) error {
	// get value from the log level cmd flag
	logLvlStr, err := cmd.Flags().GetString(FlagLogLevel)
	if err != nil {
		return err
	}

	// get value from the log format cmd flag
	logFormatStr, err := cmd.Flags().GetString(FlagLogFormat)
	if err != nil {
		return err
	}

	logLvl, err := zerolog.ParseLevel(logLvlStr)
	if err != nil {
		return err
	}

	// set the log format based on the flags
	var logWriter io.Writer
	switch strings.ToLower(logFormatStr) {

	case LogLevelJSON:
		logWriter = os.Stderr

	case LogLevelTest:
		logWriter = zerolog.ConsoleWriter{Out: os.Stderr}

	default:
		return fmt.Errorf("invalid logging format: %s", logFormatStr)
	}

	// create logger
	logger := zerolog.New(logWriter).Level(logLvl).With().Timestamp().Logger()

	// pase configurations from the config file to Config struct
	cfg, err := config.ParseConfig(args[0])
	if err != nil {
		return err
	}

	// Create context and goroutines group
	ctx, cancel := context.WithCancel(cmd.Context())
	group, ctx := errgroup.WithContext(ctx)

	// listen for and trap any OS signal to gracefully shutdown and exit
	trapSignal(cancel, logger)

	// get rpc timeout from config
	rpcTimeout, err := time.ParseDuration(cfg.RPC.RPCTimeout)
	if err != nil {
		return fmt.Errorf("failed to parse RPC timeout: %w", err)
	}

	// Gather password via env variable or std input
	skipPassword, err := cmd.Flags().GetBool(FlagSkipPassword)
	if err != nil {
		return err
	}

	keyringPass, err := getKeyringPassword(skipPassword)
	if err != nil {
		return err
	}

	// Retry creating oracle client for 5 seconds
	var oracleClient client.OracleClient
	for i := 0; i < 5; i++ {
		oracleClient, err = client.NewOracleClient(
			ctx,
			logger,
			cfg.Account.ChainID,
			cfg.Keyring.Backend,
			cfg.Keyring.Dir,
			keyringPass,
			cfg.RPC.TMRPCEndpoint,
			rpcTimeout,
			cfg.Account.Address,
			cfg.Account.Validator,
			cfg.Account.FeeGranter,
			cfg.RPC.GRPCEndpoint,
			cfg.Gas.GasAdjustment,
			cfg.Gas.GasPrices,
			cfg.Gas.GasLimit,
		)
		if err != nil {
			// sleep for a second before retrying
			time.Sleep(1 * time.Second)
			continue
		}
		break
	}

	if err != nil {
		return fmt.Errorf("error creating oracle client: %w", err)
	}

	// get provider timeout from config
	providerTimeout, err := time.ParseDuration(cfg.ProviderTimeout)
	if err != nil {
		return fmt.Errorf("failed to parse provider timeout: %w", err)
	}

	// create a map with the deviation by denom from config file
	deviations := make(map[string]math.LegacyDec, len(cfg.Deviations))
	for _, deviation := range cfg.Deviations {
		threshold, err := math.LegacyNewDecFromStr(deviation.Threshold)
		if err != nil {
			return err
		}
		deviations[deviation.Base] = threshold
	}

	// create a map with the endpoitns listed on the config file
	endpoints := make(map[string]config.ProviderEndpoint, len(cfg.ProviderEndpoints))
	for _, endpoint := range cfg.ProviderEndpoints {
		endpoints[endpoint.Name] = endpoint
	}

	// create new oracle instance
	oracle := oracle.New(
		logger,
		oracleClient,
		cfg.CurrencyPairs,
		providerTimeout,
		deviations,
		endpoints,
		cfg.Healthchecks,
	)

	// Create the telemetry config
	telemetryConfig := telemetry.Config{
		Enabled:                 cfg.Telemetry.Enabled,
		ServiceName:             cfg.Telemetry.ServiceName,
		EnableHostname:          cfg.Telemetry.EnableHostname,
		EnableHostnameLabel:     cfg.Telemetry.EnableHostnameLabel,
		EnableServiceLabel:      cfg.Telemetry.EnableServiceLabel,
		GlobalLabels:            cfg.Telemetry.GlobalLabels,
		PrometheusRetentionTime: cfg.Telemetry.PrometheusRetentionTime,
	}
	err = mapstructure.Decode(cfg.Telemetry, &telemetryConfig)
	if err != nil {
		return fmt.Errorf("failed to decode telemetry config: %w", err)
	}
	metrics, err := telemetry.New(telemetryConfig)
	if err != nil {
		return err
	}

	// Enable the services based on the config
	if cfg.Main.EnableServer {
		// Start the server
		group.Go(func() error {
			// Start the server process
			return startServer(ctx, logger, cfg, oracle, metrics)
		})
	}

	// Check if voter is enabled
	if cfg.Main.EnableVoting {
		// Start the voter process
		group.Go(func() error {
			// Start the voter process
			return startPriceOracle(ctx, logger, oracle)
		})
	}

	// Block main process until all spawned goroutines have gracefully exited and
	// signal has been captured in the main process or if an error occurs.
	return group.Wait()
}

// getKeyringPassword obtains the keyring password from the env var or stdin
func getKeyringPassword(skipPassword bool) (string, error) {
	pass := os.Getenv(envVariablePass)

	// Check if the user wants to skip the password prompt
	if skipPassword {
		return pass, nil
	}

	// Start the reader to read from stdin
	reader := bufio.NewReader(os.Stdin)

	// Check if the password is set in the environment variable
	if pass == "" {
		return input.GetString("Enter keyring password", reader)
	}
	return pass, nil
}

// trapSignal will listen for any OS signal and invoke Done on the main
// WaitGroup allowing the main process to gracefully exit.
func trapSignal(cancel context.CancelFunc, logger zerolog.Logger) {
	// create channel to store the signals
	sigCh := make(chan os.Signal, 1)

	// stay alert for SIGTERM and SIGINT signals from the OS and store on the channel
	signal.Notify(sigCh, syscall.SIGTERM)
	signal.Notify(sigCh, syscall.SIGINT)

	// launch a goroutine to handle the signal reception
	go func() {
		sig := <-sigCh // wait until the channel return a value
		logger.Info().Str("signal", sig.String()).Msg("caught signal; shutting down...")
		cancel() // execute cancel and cancel the main process
	}()
}

// startPriceOracle initialize a goroutine with the price-feeder
func startPriceOracle(ctx context.Context, logger zerolog.Logger, oracle *oracle.Oracle) error {
	// channel to receive errors from the price-feeder
	srvErrCh := make(chan error, 1)

	// launch price-feeder as goroutine
	go func() {
		logger.Info().Msg("starting price-feeder oracle...")
		srvErrCh <- oracle.Start(ctx)
	}()

	// stay tuned for errors on the context or price feeder
	for {
		select {
		case <-ctx.Done():
			logger.Info().Msg("shutting down price-feeder oracle...")
			return nil

		case err := <-srvErrCh:
			logger.Err(err).Msg("error starting the price-feeder oracle")
			oracle.Stop()
			return err
		}
	}
}

// startServer initializes a goroutine with the server
func startServer(
	ctx context.Context,
	logger zerolog.Logger,
	cfg config.Config,
	oracle *oracle.Oracle,
	metrics *telemetry.Metrics,
) error {
	// Start the router
	rtr := mux.NewRouter()
	v1Router := v1.New(logger, cfg, oracle, metrics)
	v1Router.RegisterRoutes(rtr, "")

	// Parse the read and write timeout from the config
	writeTimeout, err := time.ParseDuration(cfg.Server.WriteTimeout)
	if err != nil {
		return err
	}
	readTimeout, err := time.ParseDuration(cfg.Server.ReadTimeout)
	if err != nil {
		return err
	}

	// Create a new server with a error channel
	serverErrChannel := make(chan error, 1)
	server := &http.Server{
		Handler:           rtr,
		Addr:              cfg.Server.ListenAddress,
		WriteTimeout:      writeTimeout,
		ReadTimeout:       readTimeout,
		ReadHeaderTimeout: readTimeout,
	}

	// Start up the server in a goroutine
	go func() {
		logger.Info().Str("listen_addr", cfg.Server.ListenAddress).Msg("starting price-feeder server...")
		serverErrChannel <- server.ListenAndServe()
	}()

	// Handle states in a for loop
	for {
		select {
		case <-ctx.Done():
			// Build a shutdown context
			shutdownCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()

			// Log that the server is shutting down
			logger.Info().Str("listen_addr", cfg.Server.ListenAddress).Msg("shutting down price-feeder server...")

			// Shutdown the server
			if err := server.Shutdown(shutdownCtx); err != nil {
				logger.Err(err).Msg("error shutting down the price-feeder server")
				return err
			}

			return nil

		case err := <-serverErrChannel:
			// Log the error and return it
			logger.Error().Err(err).Msg("failed to start price-feeder server")
			return err
		}
	}
}
