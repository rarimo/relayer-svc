package cli

import (
	"context"
	"github.com/rarimo/relayer-svc/pkg/bouncer"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/rarimo/relayer-svc/internal/services"
	"github.com/rarimo/relayer-svc/internal/services/api"

	"github.com/rarimo/relayer-svc/internal/services/relayer"

	"github.com/rarimo/relayer-svc/internal/config"

	"github.com/alecthomas/kingpin"
	"gitlab.com/distributed_lab/kit/kv"
	"gitlab.com/distributed_lab/logan/v2/errors"
	"gitlab.com/distributed_lab/logan/v3"
)

func Run(args []string) {
	defer func() {
		if rvr := recover(); rvr != nil {
			logan.New().WithRecover(rvr).Fatal("app panicked")

		}
		os.Stdout.Sync()
		os.Stderr.Sync()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.New(kv.MustFromEnv())
	log := cfg.Log()

	var wg sync.WaitGroup
	run := func(f func(config.Config, context.Context)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				os.Stdout.Sync()
				os.Stderr.Sync()

				if rvr := recover(); rvr != nil {
					err := errors.FromPanic(rvr)
					logan.New().WithError(err).Fatal("one of the services panicked")
				}
			}()
			f(cfg, ctx)
		}()
	}

	app := kingpin.New("relayer-svc", "")

	runCmd := app.Command("run", "run command")
	runAllCmd := runCmd.Command("all", "")
	apiCmd := runCmd.Command("api", "run api")
	relayerCmd := runCmd.Command("relayer", "run relayer")
	generateKeyCmd := runCmd.Command("generate-key", "run generate-key")

	cmd, err := app.Parse(args[1:])
	if err != nil {
		log.WithError(err).Fatal("failed to parse arguments")
	}

	switch cmd {
	case runAllCmd.FullCommand():
		log.Info("starting all services")
		run(api.Run)
		run(services.RunScheduler)
		run(relayer.Run)
		run(services.RunQueueCleaner)
	case apiCmd.FullCommand():
		log.Info("starting API")
		run(api.Run)
	case relayerCmd.FullCommand():
		log.Info("starting relayer")
		run(relayer.Run)
		log.Info("starting queue cleaner")
		run(services.RunQueueCleaner)
	case generateKeyCmd.FullCommand():
		run(func(cfg config.Config, _ context.Context) {
			bouncer.GenerateJWT(cfg.Bouncer().Config(), cfg.Vault(), cfg.Log())
		})
	default:
		log.Fatal("unknown command %s", cmd)
	}

	var gracefulStop = make(chan os.Signal, 1)
	signal.Notify(gracefulStop, syscall.SIGTERM)
	signal.Notify(gracefulStop, syscall.SIGINT)

	// making WaitGroup usable in select
	wgch := make(chan struct{})
	go func() {
		wg.Wait()
		close(wgch)
	}()

	select {
	// listening for runners stop
	case <-wgch:
		cfg.Log().Warn("all services stopped")
	// listening for OS signals
	case <-gracefulStop:
		cfg.Log().Info("received signal to stop")
		cancel()
		<-wgch
	}
}
