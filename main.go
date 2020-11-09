package main

import (
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/options"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/logger"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/validation"
	"github.com/spf13/pflag"
)

func main() {
	logger.SetFlags(logger.Lshortfile)

	configFlagSet := pflag.NewFlagSet("oauth2-proxy", pflag.ContinueOnError)
	config := configFlagSet.String("config", "", "path to config file")
	alphaConfig := configFlagSet.String("alpha-config", "", "path to alpha config file (use at your own risk - the structure in this config file may change between minor releases)")
	showVersion := configFlagSet.Bool("version", false, "print version string")
	configFlagSet.Parse(os.Args[1:])

	if *showVersion {
		fmt.Printf("oauth2-proxy %s (built with %s)\n", VERSION, runtime.Version())
		return
	}

	opts, err := loadConfiguration(*config, *alphaConfig, configFlagSet, os.Args[1:])
	if err != nil {
		logger.Printf("ERROR: %v", err)
		os.Exit(1)
	}

	err = validation.Validate(opts)
	if err != nil {
		logger.Printf("%s", err)
		os.Exit(1)
	}

	validator := NewValidator(opts.EmailDomains, opts.AuthenticatedEmailsFile)
	oauthproxy, err := NewOAuthProxy(opts, validator)
	if err != nil {
		logger.Errorf("ERROR: Failed to initialise OAuth2 Proxy: %v", err)
		os.Exit(1)
	}

	rand.Seed(time.Now().UnixNano())

	s := &Server{
		Handler: oauthproxy,
		Opts:    opts,
		stop:    make(chan struct{}, 1),
	}
	// Observe signals in background goroutine.
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		<-sigint
		s.stop <- struct{}{} // notify having caught signal
	}()
	s.ListenAndServe()
}

// loadConfiguration will load in the user's configuration.
// It will either load the alpha configuration (if alphaConfig is given)
// or the legacy configuration.
func loadConfiguration(config, alphaConfig string, extraFlags *pflag.FlagSet, args []string) (*options.Options, error) {
	if alphaConfig != "" {
		logger.Printf("WARNING: You are using alpha configuration. The structure in this configuration file may change without notice. You MUST remove conflicting options from your existing configuration.")
		return loadAlphaOptions(config, alphaConfig, extraFlags, args)
	}
	return loadLegacyOptions(config, extraFlags, args)
}

// loadLegacyOptions loads the old toml options using the legacy flagset
// and legacy options struct.
func loadLegacyOptions(config string, extraFlags *pflag.FlagSet, args []string) (*options.Options, error) {
	optionsFlagSet := options.NewLegacyFlagSet()
	optionsFlagSet.AddFlagSet(extraFlags)
	if err := optionsFlagSet.Parse(args); err != nil {
		return nil, fmt.Errorf("failed to parse flags: %v", err)
	}

	legacyOpts := options.NewLegacyOptions()
	if err := options.Load(config, optionsFlagSet, legacyOpts); err != nil {
		return nil, fmt.Errorf("failed to load config: %v", err)
	}

	opts, err := legacyOpts.ToOptions()
	if err != nil {
		return nil, fmt.Errorf("failed to convert config: %v", err)
	}

	return opts, nil
}

// loadAlphaOptions loads the old style config excluding options converted to
// the new alpha format, then merges the alpha options, loaded from YAML,
// into the core configuration.
func loadAlphaOptions(config, alphaConfig string, extraFlags *pflag.FlagSet, args []string) (*options.Options, error) {
	opts, err := loadOptions(config, extraFlags, args)
	if err != nil {
		return nil, fmt.Errorf("failed to load core options: %v", err)
	}

	alphaOpts := &options.AlphaOptions{}
	if err := options.LoadYAML(alphaConfig, alphaOpts); err != nil {
		return nil, fmt.Errorf("failed to load alpha options: %v", err)
	}

	alphaOpts.MergeInto(opts)
	return opts, nil
}

// loadOptions loads the configuration using the old style format into the
// core options.Options struct.
// This means that none of the options that have been converted to alpha config
// will be loaded using this method.
func loadOptions(config string, extraFlags *pflag.FlagSet, args []string) (*options.Options, error) {
	optionsFlagSet := options.NewFlagSet()
	optionsFlagSet.AddFlagSet(extraFlags)
	if err := optionsFlagSet.Parse(args); err != nil {
		return nil, fmt.Errorf("failed to parse flags: %v", err)
	}

	opts := options.NewOptions()
	if err := options.Load(config, optionsFlagSet, opts); err != nil {
		return nil, fmt.Errorf("failed to load config: %v", err)
	}

	return opts, nil
}
