package main

import (
	"context"
	"flag"
	"net/http"
	"os"

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	snipsnapv1 "github.com/xgeekshq/snipsnap/api/v1"
	"github.com/xgeekshq/snipsnap/internal/controller"
	"github.com/xgeekshq/snipsnap/internal/openaiserver"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(snipsnapv1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var probeAddr string
	var apiAddr string
	var namespace string
	var workspaceName string

	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.StringVar(&apiAddr, "api-bind-address", ":8000", "The address the OpenAI-compatible API binds to.")
	flag.StringVar(&namespace, "namespace", envOrDefault("POD_NAMESPACE", "default"), "Namespace to operate in.")
	flag.StringVar(&workspaceName, "workspace-name", envOrDefault("WORKSPACE_NAME", "default"), "Name of the Workspace CR to manage.")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         false,
	})
	if err != nil {
		setupLog.Error(err, "Failed to start manager")
		os.Exit(1)
	}

	if err := (&controller.ModelReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Failed to create controller", "controller", "Model")
		os.Exit(1)
	}

	if err := (&controller.WorkspaceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Failed to create controller", "controller", "Workspace")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to set up ready check")
		os.Exit(1)
	}

	// The OpenAI API server needs a client that bypasses the cache for reads
	// so the proxy can see real-time status during model switches.
	directClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error(err, "Failed to create direct client")
		os.Exit(1)
	}

	apiHandler := openaiserver.NewHandler(directClient, namespace, workspaceName)
	apiServer := &http.Server{
		Addr:    apiAddr,
		Handler: apiHandler,
	}

	// Start the OpenAI API server in a goroutine managed by the manager lifecycle.
	if err := mgr.Add(apiServerRunnable{server: apiServer}); err != nil {
		setupLog.Error(err, "Failed to add API server to manager")
		os.Exit(1)
	}

	setupLog.Info("Starting manager", "apiAddr", apiAddr)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Failed to run manager")
		os.Exit(1)
	}
}

type apiServerRunnable struct {
	server *http.Server
}

func (a apiServerRunnable) Start(ctx context.Context) error {
	setupLog.Info("Starting OpenAI API server", "addr", a.server.Addr)

	go func() {
		<-ctx.Done()
		setupLog.Info("Shutting down OpenAI API server")
		a.server.Close()
	}()

	if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
