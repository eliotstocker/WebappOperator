// Package main is the entrypoint for the unified webapp-operator binary.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	webappv1alpha1 "github.com/eliotstocker/webapp-operator/api/v1alpha1"
	"github.com/eliotstocker/webapp-operator/internal/cache"
	"github.com/eliotstocker/webapp-operator/internal/controller"
	"github.com/eliotstocker/webapp-operator/internal/oci"
	"github.com/eliotstocker/webapp-operator/internal/server"
	"github.com/eliotstocker/webapp-operator/internal/syncer"
	"github.com/eliotstocker/webapp-operator/internal/version"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(webappv1alpha1.AddToScheme(scheme))
	utilruntime.Must(networkingv1.AddToScheme(scheme))
	utilruntime.Must(discoveryv1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(coordinationv1.AddToScheme(scheme))
}

func main() {
	var httpAddr string
	var cacheDir string
	var serviceName string
	var servicePort int
	var operatorNamespace string
	var enableLeaderElection bool
	var printVersion bool

	flag.StringVar(&httpAddr, "http-bind-address", ":8080", "The address the unified HTTP server binds to.")
	flag.StringVar(&cacheDir, "cache-dir", "/tmp/webapp-cache", "The directory where OCI layers are unpacked.")
	flag.StringVar(&serviceName, "service-name", "webapp-operator", "The Kubernetes service name for ingress backends.")
	flag.IntVar(&servicePort, "service-port", 80, "The Kubernetes service port for ingress backends.")
	flag.StringVar(&operatorNamespace, "operator-namespace", "default", "Namespace where operator pods are running.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", true, "Enable leader election for controller manager.")
	flag.BoolVar(&printVersion, "version", false, "Print version and exit.")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	if printVersion {
		fmt.Printf("webapp-operator version %s\n", version.Version)
		os.Exit(0)
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		operatorNamespace = ns
	}
	podName := os.Getenv("POD_NAME")
	if podName == "" {
		podName = "local-dev"
	}

	cacheMgr, err := cache.NewDiskManager(cacheDir)
	if err != nil {
		setupLog.Error(err, "unable to initialize cache manager")
		os.Exit(1)
	}

	staticRouter := server.NewRouter(cacheMgr)
	syncTracker := syncer.NewTracker()

	// Unified HTTP multiplexer: single port for traffic, health, and peer sync
	rootMux := http.NewServeMux()
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	rootMux.Handle("/healthz", okHandler)
	rootMux.Handle("/readyz", okHandler)
	rootMux.Handle("/internal/sync", syncTracker)
	rootMux.Handle("/", staticRouter)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "webapp-operator-leader.webapp.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	var isLeaderAtomic atomic.Bool
	if !enableLeaderElection {
		isLeaderAtomic.Store(true)
	}

	// Run the single HTTP server
	if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		go func() {
			select {
			case <-mgr.Elected():
				isLeaderAtomic.Store(true)
				setupLog.Info("Pod elected as cluster Leader")
			case <-ctx.Done():
			}
		}()

		srv := &http.Server{Addr: httpAddr, Handler: rootMux}
		go func() {
			<-ctx.Done()
			_ = srv.Shutdown(context.Background())
		}()
		setupLog.Info("Starting unified HTTP server", "addr", httpAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("http server error: %w", err)
		}
		return nil
	})); err != nil {
		setupLog.Error(err, "unable to add HTTP server to manager")
		os.Exit(1)
	}

	if err = (&controller.WebsiteReconciler{
		Client:       mgr.GetClient(),
		Scheme:       mgr.GetScheme(),
		Router:       staticRouter,
		CacheManager: cacheMgr,
		ServiceName:  serviceName,
		ServicePort:  int32(servicePort),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Website")
		os.Exit(1)
	}

	if err = (&controller.WebsiteRevisionReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		CacheManager:      cacheMgr,
		Puller:            oci.NewPuller(),
		SyncerClient:      syncer.NewClient(),
		Tracker:           syncTracker,
		Router:            staticRouter,
		PodName:           podName,
		OperatorNamespace: operatorNamespace,
		ServiceName:       serviceName,
		IsLeader:          isLeaderAtomic.Load,
		GetLeaderAddress: func(ctx context.Context) (string, error) {
			var lease coordinationv1.Lease
			if err := mgr.GetClient().Get(ctx, client.ObjectKey{Namespace: operatorNamespace, Name: "webapp-operator-leader.webapp.io"}, &lease); err != nil {
				return "", err
			}
			if lease.Spec.HolderIdentity == nil {
				return "", fmt.Errorf("no leader elected")
			}
			leaderPod := strings.Split(*lease.Spec.HolderIdentity, "_")[0]

			var epList discoveryv1.EndpointSliceList
			listOpts := []client.ListOption{client.InNamespace(operatorNamespace)}
			if serviceName != "" {
				listOpts = append(listOpts, client.MatchingLabels{discoveryv1.LabelServiceName: serviceName})
			}
			if err := mgr.GetClient().List(ctx, &epList, listOpts...); err != nil {
				return "", err
			}
			for _, slice := range epList.Items {
				for _, ep := range slice.Endpoints {
					if ep.TargetRef != nil && ep.TargetRef.Name == leaderPod && len(ep.Addresses) > 0 {
						return net.JoinHostPort(ep.Addresses[0], "8080"), nil
					}
				}
			}
			return "", fmt.Errorf("leader pod %q not found in EndpointSlices", leaderPod)
		},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "WebsiteRevision")
		os.Exit(1)
	}

	setupLog.Info("Starting webapp-operator unified manager", "version", version.Version)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
