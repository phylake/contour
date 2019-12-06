// Copyright © 2019 VMware
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"reflect"
	"strconv"
	"syscall"
	"time"

	contourinformers "github.com/projectcontour/contour/apis/generated/informers/externalversions"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/debug"
	cgrpc "github.com/projectcontour/contour/internal/grpc"
	"github.com/projectcontour/contour/internal/httpsvc"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/metrics"
	"github.com/projectcontour/contour/internal/workgroup"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v2"
	coreinformers "k8s.io/client-go/informers"
)

// registerServe registers the serve subcommand and flags
// with the Application provided.
func registerServe(app *kingpin.Application) (*kingpin.CmdClause, *serveContext) {
	serve := app.Command("serve", "Serve xDS API traffic")

	// The precedence of configuration for contour serve is as follows:
	// config file, overridden by env vars, overridden by cli flags.
	// however, as -c is a cli flag, we don't know its valye til cli flags
	// have been parsed. To correct this ordering we assign a post parse
	// action to -c, then parse cli flags twice (see main.main). On the second
	// parse our action will return early, resulting in the precedence order
	// we want.
	var (
		configFile string
		parsed     bool
	)
	ctx := newServeContext()

	parseConfig := func(_ *kingpin.ParseContext) error {
		if parsed || configFile == "" {
			// if there is no config file supplied, or we've
			// already parsed it, return immediately.
			return nil
		}
		f, err := os.Open(configFile)
		if err != nil {
			return err
		}
		defer f.Close()
		dec := yaml.NewDecoder(f)
		parsed = true
		return dec.Decode(&ctx)
	}

	serve.Flag("config-path", "path to base configuration").Short('c').Action(parseConfig).ExistingFileVar(&configFile)

	serve.Flag("incluster", "use in cluster configuration.").BoolVar(&ctx.InCluster)
	serve.Flag("kubeconfig", "path to kubeconfig (if not in running inside a cluster)").StringVar(&ctx.Kubeconfig)

	serve.Flag("xds-address", "xDS gRPC API address").StringVar(&ctx.xdsAddr)
	serve.Flag("xds-port", "xDS gRPC API port").IntVar(&ctx.xdsPort)

	serve.Flag("stats-address", "Envoy /stats interface address").StringVar(&ctx.statsAddr)
	serve.Flag("stats-port", "Envoy /stats interface port").IntVar(&ctx.statsPort)

	serve.Flag("debug-http-address", "address the debug http endpoint will bind to").StringVar(&ctx.debugAddr)
	serve.Flag("debug-http-port", "port the debug http endpoint will bind to").IntVar(&ctx.debugPort)

	serve.Flag("http-address", "address the metrics http endpoint will bind to").StringVar(&ctx.metricsAddr)
	serve.Flag("http-port", "port the metrics http endpoint will bind to").IntVar(&ctx.metricsPort)

	serve.Flag("contour-cafile", "CA bundle file name for serving gRPC with TLS").Envar("CONTOUR_CAFILE").StringVar(&ctx.caFile)
	serve.Flag("contour-cert-file", "Contour certificate file name for serving gRPC over TLS").Envar("CONTOUR_CERT_FILE").StringVar(&ctx.contourCert)
	serve.Flag("contour-key-file", "Contour key file name for serving gRPC over TLS").Envar("CONTOUR_KEY_FILE").StringVar(&ctx.contourKey)
	serve.Flag("insecure", "Allow serving without TLS secured gRPC").BoolVar(&ctx.PermitInsecureGRPC)
	// TODO(sas) Deprecate `ingressroute-root-namespaces` in v1.0
	serve.Flag("ingressroute-root-namespaces", "DEPRECATED (Use 'root-namespaces'): Restrict contour to searching these namespaces for root ingress routes").StringVar(&ctx.rootNamespaces)
	serve.Flag("root-namespaces", "Restrict contour to searching these namespaces for root ingress routes").StringVar(&ctx.rootNamespaces)

	serve.Flag("ingress-class-name", "Contour IngressClass name").StringVar(&ctx.ingressClass)

	serve.Flag("envoy-http-access-log", "Envoy HTTP access log").StringVar(&ctx.httpAccessLog)
	serve.Flag("envoy-https-access-log", "Envoy HTTPS access log").StringVar(&ctx.httpsAccessLog)
	serve.Flag("envoy-service-http-address", "Kubernetes Service address for HTTP requests").StringVar(&ctx.httpAddr)
	serve.Flag("envoy-service-https-address", "Kubernetes Service address for HTTPS requests").StringVar(&ctx.httpsAddr)
	serve.Flag("envoy-service-http-port", "Kubernetes Service port for HTTP requests").IntVar(&ctx.httpPort)
	serve.Flag("envoy-service-https-port", "Kubernetes Service port for HTTPS requests").IntVar(&ctx.httpsPort)
	serve.Flag("use-proxy-protocol", "Use PROXY protocol for all listeners").BoolVar(&ctx.useProxyProto)

	serve.Flag("accesslog-format", "Format for Envoy access logs").StringVar(&ctx.AccessLogFormat)
	serve.Flag("disable-leader-election", "Disable leader election mechanism").BoolVar(&ctx.DisableLeaderElection)
	return serve, ctx
}

// doServe runs the contour serve subcommand.
func doServe(log logrus.FieldLogger, ctx *serveContext) error {

	// step 1. establish k8s client connection
	client, contourClient, coordinationClient := newClient(ctx.Kubeconfig, ctx.InCluster)

	// step 2. create informers
	// note: 0 means resync timers are disabled
	coreInformers := coreinformers.NewSharedInformerFactory(client, 0)
	contourInformers := contourinformers.NewSharedInformerFactory(contourClient, 0)

	// Create a set of SharedInformerFactories for each root-ingressroute namespace (if defined)
	var namespacedInformers []coreinformers.SharedInformerFactory
	for _, namespace := range ctx.ingressRouteRootNamespaces() {
		inf := coreinformers.NewSharedInformerFactoryWithOptions(client, 0, coreinformers.WithNamespace(namespace))
		namespacedInformers = append(namespacedInformers, inf)
	}

	// step 3. build our mammoth Kubernetes event handler.
	eh := &contour.EventHandler{
		CacheHandler: &contour.CacheHandler{
			ListenerVisitorConfig: contour.ListenerVisitorConfig{
				UseProxyProto:          ctx.useProxyProto,
				HTTPAddress:            ctx.httpAddr,
				HTTPPort:               ctx.httpPort,
				HTTPAccessLog:          ctx.httpAccessLog,
				HTTPSAddress:           ctx.httpsAddr,
				HTTPSPort:              ctx.httpsPort,
				HTTPSAccessLog:         ctx.httpsAccessLog,
				AccessLogType:          ctx.AccessLogFormat,
				AccessLogFields:        ctx.AccessLogFields,
				MinimumProtocolVersion: dag.MinProtoVersion(ctx.TLSConfig.MinimumProtocolVersion),
			},
			ListenerCache: contour.NewListenerCache(ctx.statsAddr, ctx.statsPort),
			FieldLogger:   log.WithField("context", "CacheHandler"),
		},
		HoldoffDelay:    100 * time.Millisecond,
		HoldoffMaxDelay: 500 * time.Millisecond,
		CRDStatus: &k8s.CRDStatus{
			Client: contourClient,
		},
		Builder: dag.Builder{
			Source: dag.KubernetesCache{
				RootNamespaces: ctx.ingressRouteRootNamespaces(),
				IngressClass:   ctx.ingressClass,
				FieldLogger:    log.WithField("context", "KubernetesCache"),
			},
			DisablePermitInsecure: ctx.DisablePermitInsecure,
		},
		FieldLogger: log.WithField("context", "contourEventHandler"),
	}

	// step 4. register our resource event handler with the k8s informers.
	coreInformers.Core().V1().Services().Informer().AddEventHandler(eh)
	coreInformers.Extensions().V1beta1().Ingresses().Informer().AddEventHandler(eh)
	contourInformers.Contour().V1beta1().IngressRoutes().Informer().AddEventHandler(eh)
	contourInformers.Contour().V1beta1().TLSCertificateDelegations().Informer().AddEventHandler(eh)
	contourInformers.Projectcontour().V1alpha1().HTTPProxies().Informer().AddEventHandler(eh)
	contourInformers.Projectcontour().V1alpha1().TLSCertificateDelegations().Informer().AddEventHandler(eh)

	// Add informers for each root-ingressroute namespaces
	for _, inf := range namespacedInformers {
		inf.Core().V1().Secrets().Informer().AddEventHandler(eh)
	}
	// If root-ingressroutes are not defined, then add the informer for all namespaces
	if len(namespacedInformers) == 0 {
		coreInformers.Core().V1().Secrets().Informer().AddEventHandler(eh)
	}

	// step 5. endpoints updates are handled directly by the EndpointsTranslator
	// due to their high update rate and their orthogonal nature.
	et := &contour.EndpointsTranslator{
		FieldLogger: log.WithField("context", "endpointstranslator"),
	}
	coreInformers.Core().V1().Endpoints().Informer().AddEventHandler(et)

	// step 6. setup workgroup runner and register informers.
	var g workgroup.Group
	g.Add(startInformer(coreInformers, log.WithField("context", "coreinformers")))
	g.Add(startInformer(contourInformers, log.WithField("context", "contourinformers")))
	for _, inf := range namespacedInformers {
		g.Add(startInformer(inf, log.WithField("context", "corenamespacedinformers")))
	}

	// step 7. register our event handler with the workgroup
	g.Add(eh.Start())

	// step 8. setup prometheus registry and register base metrics.
	registry := prometheus.NewRegistry()
	registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	registry.MustRegister(prometheus.NewGoCollector())

	// step 9. create metrics service and register with workgroup.
	metricsvc := metrics.Service{
		Service: httpsvc.Service{
			Addr:        ctx.metricsAddr,
			Port:        ctx.metricsPort,
			FieldLogger: log.WithField("context", "metricsvc"),
		},
		Client:   client,
		Registry: registry,
	}
	g.Add(metricsvc.Start)

	// step 10. create debug service and register with workgroup.
	debugsvc := debug.Service{
		Service: httpsvc.Service{
			Addr:        ctx.debugAddr,
			Port:        ctx.debugPort,
			FieldLogger: log.WithField("context", "debugsvc"),
		},
		Builder: &eh.Builder,
	}
	g.Add(debugsvc.Start)

	// step 11. if enabled, register leader election
	if !ctx.DisableLeaderElection {
		log := log.WithField("context", "leaderelection")
		le, _, deposed := newLeaderElector(log, ctx, client, coordinationClient)

		g.AddContext(func(electionCtx context.Context) {
			log.WithFields(logrus.Fields{
				"configmapname":      ctx.LeaderElectionConfig.Name,
				"configmapnamespace": ctx.LeaderElectionConfig.Namespace,
			}).Info("started")

			le.Run(electionCtx)
			log.Info("stopped")
		})

		g.Add(func(stop <-chan struct{}) error {
			// If we get deposed as leader, shut it down.
			log := log.WithField("context", "leaderelection-deposer")
			select {
			case <-stop:
				// shut down
				log.Info("stopped")
			case <-deposed:
				log.Info("deposed as leader, shutting down")
			}
			return nil
		})
	} else {
		log.Info("Leader election disabled")
	}

	// step 12. register our custom metrics and plumb into cache handler
	// and resource event handler.
	metrics := metrics.NewMetrics(registry)
	eh.Metrics = metrics
	eh.CacheHandler.Metrics = metrics

	// step 12.5. synchronous cache init (Adobe)
	err = initCache(client, contourClient, eh, et)
	check(err)

	// step 13. create grpc handler and register with workgroup.
	g.Add(func(stop <-chan struct{}) error {
		log := log.WithField("context", "grpc")
		resources := map[string]cgrpc.Resource{
			eh.CacheHandler.ClusterCache.TypeURL():  &eh.CacheHandler.ClusterCache,
			eh.CacheHandler.RouteCache.TypeURL():    &eh.CacheHandler.RouteCache,
			eh.CacheHandler.ListenerCache.TypeURL(): &eh.CacheHandler.ListenerCache,
			eh.CacheHandler.SecretCache.TypeURL():   &eh.CacheHandler.SecretCache,
			et.TypeURL():                            et,
		}
		opts := ctx.grpcOptions()
		s := cgrpc.NewAPI(log, resources, opts...)
		addr := net.JoinHostPort(ctx.xdsAddr, strconv.Itoa(ctx.xdsPort))
		l, err := net.Listen("tcp", addr)
		if err != nil {
			return err
		}

		log = log.WithField("address", addr)
		if ctx.PermitInsecureGRPC {
			log = log.WithField("insecure", true)
		}

		log.Info("started")
		defer log.Info("stopped")

		go func() {
			<-stop
			s.Stop()
		}()

		return s.Serve(l)
	})

	// step 14. Setup SIGTERM handler
	g.Add(func(stop <-chan struct{}) error {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM)
		select {
		case <-c:
			log.WithField("context", "sigterm-handler").Info("received SIGTERM, shutting down")
		case <-stop:
			// Do nothing. The group is shutting down.
		}
		return nil
	})

	// step 15. GO!
	return g.Run()
}

type informer interface {
	WaitForCacheSync(stopCh <-chan struct{}) map[reflect.Type]bool
	Start(stopCh <-chan struct{})
}

func startInformer(inf informer, log logrus.FieldLogger) func(stop <-chan struct{}) error {
	return func(stop <-chan struct{}) error {
		log.Println("waiting for cache sync")
		inf.WaitForCacheSync(stop)

		log.Println("started")
		defer log.Println("stopped")
		inf.Start(stop)
		<-stop
		return nil
	}
}
