package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"code.cloudfoundry.org/debugserver"
	"code.cloudfoundry.org/go-db-helpers/marshal"
	"code.cloudfoundry.org/go-db-helpers/mutualtls"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/controller/config"
	"code.cloudfoundry.org/silk/controller/handlers"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"github.com/tedsuo/ifrit/http_server"
	"github.com/tedsuo/ifrit/sigmon"
)

func main() {
	if err := mainWithError(); err != nil {
		log.Fatalf("silk-controller error: %s", err)
	}
}

func mainWithError() error {
	logger := lager.NewLogger("silk-controller")
	reconfigurableSink := lager.NewReconfigurableSink(
		lager.NewWriterSink(os.Stdout, lager.DEBUG),
		lager.INFO)
	logger.RegisterSink(reconfigurableSink)
	logger.Info("starting")

	var configFilePath string
	flag.StringVar(&configFilePath, "config-file", "", "path to config file")
	flag.Parse()

	conf, err := config.ReadFromFile(configFilePath)
	if err != nil {
		return fmt.Errorf("loading config: %s", err)
	}

	debugServerAddress := fmt.Sprintf("127.0.0.1:%d", conf.DebugServerPort)
	mainServerAddress := fmt.Sprintf("%s:%d", conf.ListenHost, conf.ListenPort)
	tlsConfig, err := mutualtls.NewServerTLSConfig(conf.ServerCertFile, conf.ServerKeyFile, conf.CACertFile)
	if err != nil {
		log.Fatalf("mutual tls config: %s", err)
	}

	leasesIndex := &handlers.LeasesIndex{
		Logger:    logger,
		Marshaler: marshal.MarshalFunc(json.Marshal),
	}

	httpServer := http_server.NewTLSServer(mainServerAddress, leasesIndex, tlsConfig)
	members := grouper.Members{
		{"http_server", httpServer},
		{"debug-server", debugserver.Runner(debugServerAddress, reconfigurableSink)},
	}

	group := grouper.NewOrdered(os.Interrupt, members)
	monitor := ifrit.Invoke(sigmon.New(group))

	err = <-monitor.Wait()
	if err != nil {
		return fmt.Errorf("wait returned error: %s", err)
	}

	logger.Info("exited")
	return nil
}