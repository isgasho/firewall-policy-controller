package main

import (
	"fmt"
	"io/ioutil"

	"github.com/ghodss/yaml"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"os"

	"go.uber.org/zap"

	controller "git.f-i-ts.de/cloud-native/firewall-policy-controller/pkg/controller"
	"git.f-i-ts.de/cloud-native/firewall-policy-controller/pkg/watcher"
	"git.f-i-ts.de/cloud-native/metallib/version"
	"git.f-i-ts.de/cloud-native/metallib/zapup"

	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	moduleName = "firewall-policy-controller"
)

var (
	logger = zapup.MustRootLogger().Sugar()
	debug  = false
)

var rootCmd = &cobra.Command{
	Use:     moduleName,
	Short:   "a service that assembles and enforces firewall rules based on k8s resources",
	Version: version.V.String(),
	Run: func(cmd *cobra.Command, args []string) {
		debug = logger.Desugar().Core().Enabled(zap.DebugLevel)
		run()
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		logger.Error("failed executing root command", "error", err)
	}
}

func init() {
	viper.SetEnvPrefix("FIREWALL_")
	homedir, err := homedir.Dir()
	if err != nil {
		logger.Fatal(err)
	}
	rootCmd.PersistentFlags().StringP("kubecfg", "k", homedir+"/.kube/config", "kubecfg path to the cluster to account")
	viper.BindPFlags(rootCmd.PersistentFlags())
}

func run() {
	client, err := loadClient(viper.GetString("kubecfg"))
	if err != nil {
		logger.Errorw("unable to connect to k8s", "error", err)
		os.Exit(1)
	}
	ctr := controller.NewFirewallController(client, logger)
	c := make(chan bool)
	svcWatcher := watcher.NewServiceWatcher(logger, client)
	npWatcher := watcher.NewNetworkPolicyWatcher(logger, client)
	go svcWatcher.Watch(c)
	go npWatcher.Watch(c)

	for <-c {
		rules, err := ctr.FetchAndAssemble()
		if err != nil {
			logger.Errorw("could not fetch k8s entities to build firewall rules", "error", err)
		}
		logger.Infow("new fw rules", "rules", rules.ToString())
	}
}

func loadClient(kubeconfigPath string) (*k8s.Clientset, error) {
	data, err := ioutil.ReadFile(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("read kubeconfig: %v", err)
	}
	var config rest.Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("unmarshal kubeconfig: %v", err)
	}
	return k8s.NewForConfig(&config)
}
