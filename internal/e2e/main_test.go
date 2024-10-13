package e2e

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/benfiola/external-dns-routeros-provider/internal/provider"
	"github.com/go-routeros/routeros/v3"
	"github.com/neilotoole/slogt"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
	"sigs.k8s.io/external-dns/controller"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/pkg/apis/externaldns"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider/webhook"
	"sigs.k8s.io/external-dns/registry"
	"sigs.k8s.io/external-dns/source"
)

var (
	testObjects = []client.Object{}
	logrusHook  *LogrusHook
)

// Adds a test object to a test objects list.
// This list is used to help during between-test cleanup.
// See: [Setup]
func CreateTestObject[T client.Object](c T) T {
	testObjects = append(testObjects, c)
	return c
}

// Defines kubernetes objects referenced during tests.
// Using a static set of tests ensures that cleanup is consistent between test runs.
// The tests themselves should create these objects as needed.
// [Setup] handles the cleanup of these resources.
// NOTE: Ensure matching routeros objects are removed in [Setup].
var (
	aRecord = CreateTestObject(&endpoint.DNSEndpoint{
		ObjectMeta: v1.ObjectMeta{Namespace: "default", Name: "a"},
		Spec:       endpoint.DNSEndpointSpec{Endpoints: []*endpoint.Endpoint{endpoint.NewEndpoint("a", "A", "127.0.0.1")}},
	})
	cnameRecord = CreateTestObject(&endpoint.DNSEndpoint{
		ObjectMeta: v1.ObjectMeta{Namespace: "default", Name: "cname"},
		Spec:       endpoint.DNSEndpointSpec{Endpoints: []*endpoint.Endpoint{endpoint.NewEndpoint("cname.testing", "CNAME", "original.testing")}},
	})
	mxRecord = CreateTestObject(&endpoint.DNSEndpoint{
		ObjectMeta: v1.ObjectMeta{Namespace: "default", Name: "mx"},
		Spec:       endpoint.DNSEndpointSpec{Endpoints: []*endpoint.Endpoint{endpoint.NewEndpoint("mx.testing", "MX", "0 exchange.testing")}},
	})
	nsRecord = CreateTestObject(&endpoint.DNSEndpoint{
		ObjectMeta: v1.ObjectMeta{Namespace: "default", Name: "ns"},
		Spec:       endpoint.DNSEndpointSpec{Endpoints: []*endpoint.Endpoint{endpoint.NewEndpoint("ns.testing", "NS", "nameserver.testing")}},
	})
	srvRecord = CreateTestObject(&endpoint.DNSEndpoint{
		ObjectMeta: v1.ObjectMeta{Namespace: "default", Name: "src"},
		Spec:       endpoint.DNSEndpointSpec{Endpoints: []*endpoint.Endpoint{endpoint.NewEndpoint("srv.testing", "SRV", "0 0 80 original.testing")}},
	})
	txtRecord = CreateTestObject(&endpoint.DNSEndpoint{
		ObjectMeta: v1.ObjectMeta{Namespace: "default", Name: "txt"},
		Spec:       endpoint.DNSEndpointSpec{Endpoints: []*endpoint.Endpoint{endpoint.NewEndpoint("txt.testing", "TXT", "text record")}},
	})
)

// TestData holds data used during tests
// See [Setup].
type TestData struct {
	Ctx        context.Context
	Kubeconfig string
	Kube       client.Client
	Require    *require.Assertions
	Routeros   *routeros.Client
	T          testing.TB
}

// Cleans up existing test objects and removes resources from routeros.
// Returns a set of data used across most (all?) tests.
func Setup(t testing.TB) TestData {
	t.Helper()

	require := require.New(t)
	ctx := context.Background()

	// create kube client
	kc := os.Getenv("KUBECONFIG")
	rcfg, err := clientcmd.BuildConfigFromFlags("", kc)
	require.NoError(err, "build client config")
	s := runtime.NewScheme()
	err = kscheme.AddToScheme(s)
	require.NoError(err, "add kubernetes resources to scheme")
	edgv := schema.GroupVersion{Group: "externaldns.k8s.io", Version: "v1alpha1"}
	edsb := &scheme.Builder{GroupVersion: edgv}
	edsb.Register(&endpoint.DNSEndpoint{})
	err = edsb.AddToScheme(s)
	require.NoError(err, "add externaldns resources to scheme")
	k, err := client.New(rcfg, client.Options{Scheme: s, Cache: &client.CacheOptions{}})
	require.NoError(err, "build client")

	// delete existing kubernetes resources
	for _, to := range testObjects {
		cto := to.DeepCopyObject().(client.Object)
		err := k.Delete(ctx, cto)
		if apierrors.IsNotFound(err) {
			err = nil
		}
		require.NoError(err, "clean up resource")
	}

	// create routeros client
	ros, err := routeros.Dial("127.0.0.1:8728", "admin", "")
	require.NoError(err, "create routeros client")

	// ensure routeros client gets closed when test finishes
	t.Cleanup(func() {
		err := ros.Close()
		require.NoError(err, "close routeros client")
	})

	// delete all routeros dns records
	rs, err := ros.RunArgs([]string{"/ip/dns/static/print", "=detail"})
	require.NoError(err, "list routeros dns records")
	for _, re := range rs.Re {
		_, err := ros.RunArgs([]string{"/ip/dns/static/remove", fmt.Sprintf("=.id=%s", re.Map[".id"])})
		require.NoError(err, "delete routeros dns record")
	}

	return TestData{
		Ctx:        ctx,
		Kubeconfig: kc,
		Kube:       k,
		Require:    require,
		Routeros:   ros,
		T:          t,
	}
}

// LogrusHook is a hook that routes logrus logging behavior to a [slog.Logger] instead.
// See: [CreateController]
// See: [logrusHook]
type LogrusHook struct {
	Logger *slog.Logger
}

// Implements the [logrus.Hook] interface
func (lh *LogrusHook) Levels() []logrus.Level { return logrus.AllLevels }

// Implements the [logrus.Hook] interface
func (lh *LogrusHook) Fire(e *logrus.Entry) error {
	switch e.Level {
	case logrus.PanicLevel:
	case logrus.FatalLevel:
	case logrus.ErrorLevel:
		lh.Logger.Error(e.Message)
	case logrus.WarnLevel:
		lh.Logger.Warn(e.Message)
	case logrus.InfoLevel:
		lh.Logger.Info(e.Message)
	case logrus.DebugLevel:
		lh.Logger.Debug(e.Message)
	}
	return nil
}

// Given [TestData], creates an instance of a [controller.Controller].
// Used only in [RunControllerUntil] - however, creating a controller is arduous enough that it's put inside this helper method.
func CreateController(td TestData) *controller.Controller {
	td.T.Helper()

	// the provider uses a global logrus logger
	// configure the global logrus logger to log to [io.Discard]
	// install a hook that routes logs to the current test's logger.
	l := slogt.New(td.T).With("name", "controller")
	if logrusHook == nil {
		logrusHook = &LogrusHook{Logger: l}
		logrus.SetOutput(io.Discard)
		logrus.AddHook(logrusHook)
	}
	logrusHook.Logger = l

	cfg := externaldns.NewConfig()
	cfg.ParseFlags([]string{})
	cfg.Interval = time.Duration(50 * time.Millisecond)
	cfg.LogLevel = "debug"
	cfg.KubeConfig = td.Kubeconfig
	cfg.ManagedDNSRecordTypes = []string{"A", "CNAME", "MX", "NS", "SRV", "TXT"}
	cfg.Sources = []string{"crd"}

	lf, _ := labels.Parse(cfg.LabelFilter)
	scfg := source.Config{
		APIServerURL:        cfg.APIServerURL,
		CRDSourceAPIVersion: cfg.CRDSourceAPIVersion,
		CRDSourceKind:       cfg.CRDSourceKind,
		DefaultTargets:      cfg.DefaultTargets,
		KubeConfig:          cfg.KubeConfig,
		LabelFilter:         lf,
	}
	ss, err := source.ByNames(td.Ctx, &source.SingletonClientGenerator{
		KubeConfig: cfg.KubeConfig,
	}, cfg.Sources, &scfg)
	td.Require.NoError(err, "create controller source")
	s := source.NewMultiSource(ss, cfg.DefaultTargets)

	prov, err := webhook.NewWebhookProvider(cfg.WebhookProviderURL)
	td.Require.NoError(err, "create controller webhook provider")

	r, err := registry.NewTXTRegistry(
		prov,
		cfg.TXTPrefix,
		cfg.TXTSuffix,
		cfg.TXTOwnerID,
		cfg.TXTCacheInterval,
		cfg.TXTWildcardReplacement,
		cfg.ManagedDNSRecordTypes,
		cfg.ExcludeDNSRecordTypes,
		cfg.TXTEncryptEnabled,
		[]byte(cfg.TXTEncryptAESKey),
	)
	td.Require.NoError(err, "create controller registry")

	c := controller.Controller{
		Source:               s,
		Registry:             r,
		Policy:               plan.Policies[cfg.Policy],
		Interval:             cfg.Interval,
		DomainFilter:         endpoint.NewDomainFilter([]string{}),
		ManagedRecordTypes:   cfg.ManagedDNSRecordTypes,
		ExcludeRecordTypes:   cfg.ExcludeDNSRecordTypes,
		MinEventSyncInterval: cfg.MinEventSyncInterval,
	}
	return &c
}

// StopIteration signals to [RunUntil] that iteration should be stopped
type StopIteration struct{}

// Implements the [error] interface by returning an error string.
func (si StopIteration) Error() string { return "stop iteration" }

// Runs external-dns controller (+ provider) until provided function returns [StopIteration].
func RunControllerUntil(td TestData, cb func() error) {
	td.T.Helper()

	l := slogt.New(td.T)

	sctx, cancel := context.WithCancel(td.Ctx)

	// create webhook
	w, err := provider.New(&provider.Opts{Logger: l, RouterOSAddress: "127.0.0.1:8728", RouterOSUsername: "admin"})
	td.Require.NoError(err, "create provider")

	// create shared data
	var werr error
	var wg sync.WaitGroup

	// ensure context cancelled, goroutines terminate
	defer func() {
		cancel()
		wg.Wait()
	}()

	// start webhook
	wg.Add(1)
	go func() {
		defer wg.Done()
		werr = w.Run(sctx)
	}()

	// wait for webhook to be ready
	n := time.Now()
	to := 5 * time.Second
	for {
		if time.Since(n) > to {
			err = fmt.Errorf("timeout reached")
			break
		}
		_, err := http.Get("http://127.0.0.1:8888/healthz")
		not_ready := false
		if err != nil {
			if errors.Is(err, syscall.ECONNREFUSED) {
				not_ready = true
			}
		}
		if not_ready {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		break
	}
	td.Require.NoError(err, "wait for webhook to be ready")

	// create controller
	c := CreateController(td)

	// start controller
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.Run(sctx)
	}()

	// poll until condition reached
	n = time.Now()
	to = 60 * time.Second
	for {
		if time.Since(n) > to {
			err = fmt.Errorf("timed out")
			break
		}
		if err != nil {
			break
		}
		err := cb()
		if err != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if _, ok := err.(StopIteration); ok {
		err = nil
	}
	td.Require.NoError(err, "fail while polling")
	td.Require.NoError(werr, "fail from provider")
}

// Finds routeros records matching the given [endpoint.DNSEndpoint]
// See: [WaitForSync]
// See: [WaitForDelete]
func FindRecords(td TestData, de *endpoint.DNSEndpoint) []map[string]string {
	td.T.Helper()

	r, err := td.Routeros.Run("/ip/dns/static/print", "=detail")
	td.Require.NoError(err, "list routeros dns records")

	d := []map[string]string{}

	for _, e := range de.Spec.Endpoints {
		ettl := e.RecordTTL
		if ettl == 0 {
			// provider sets a default ttl of 1d when unset
			ettl = endpoint.TTL((24 * time.Hour).Seconds())
		}

		for _, re := range r.Re {
			r := re.Map

			rttl, err := provider.ParseRouterOSDuration(r["ttl"])
			td.Require.NoError(err, "convert routeros ttl %s", r["ttl"])

			if e.RecordType != r["type"] || e.DNSName != r["name"] || ettl != rttl {
				continue
			}

			var t string
			switch r["type"] {
			case "A":
				t = r["address"]
			case "CNAME":
				t = r["cname"]
			case "MX":
				t = fmt.Sprintf("%s %s", r["mx-preference"], r["mx-exchange"])
			case "NS":
				t = r["ns"]
			case "SRV":
				t = fmt.Sprintf("%s %s %s %s", r["srv-priority"], r["srv-weight"], r["srv-port"], r["srv-target"])
			case "TXT":
				t = r["text"]
			default:
				continue
			}

			if !slices.Contains(e.Targets, t) {
				continue
			}

			d = append(d, r)
		}
	}

	return d
}

// Runs the controller until the provided [endpoint.DNSEndpoint] has a matching set of routeros records.
func WaitForSync(td TestData, de *endpoint.DNSEndpoint) {
	c := 0
	for _, e := range de.Spec.Endpoints {
		c += len(e.Targets)
	}

	RunControllerUntil(td, func() error {
		rs := FindRecords(td, de)
		if len(rs) != c {
			return nil
		}
		return StopIteration{}
	})

	err := td.Kube.Get(td.Ctx, client.ObjectKeyFromObject(de), de)
	td.Require.NoError(err, "refresh dns endpoint")
}

// Runs the controller until the provided [endpoint.DNSEndpoint] has no matching DNS records.
func WaitForDelete(td TestData, de *endpoint.DNSEndpoint) {
	RunControllerUntil(td, func() error {
		rs := FindRecords(td, de)
		if len(rs) != 0 {
			return nil
		}
		return StopIteration{}
	})
}

func TestARecord(t *testing.T) {
	createARecord := func(td TestData) *endpoint.DNSEndpoint {
		td.T.Helper()

		a := aRecord.DeepCopy()
		err := td.Kube.Create(td.Ctx, a)
		td.Require.NoError(err, "create A record")

		return a
	}

	t.Run("create", func(t *testing.T) {
		td := Setup(t)

		a := createARecord(td)
		WaitForSync(td, a)
	})

	t.Run("update", func(t *testing.T) {
		td := Setup(t)

		a := createARecord(td)
		WaitForSync(td, a)

		a.Spec.Endpoints[0].Targets[0] = "8.8.8.8"
		a.Spec.Endpoints[0].RecordTTL = endpoint.TTL(60 * 60 * 4)
		err := td.Kube.Update(td.Ctx, a)
		td.Require.NoError(err, "update A record")
		WaitForSync(td, a)
	})

	t.Run("delete", func(t *testing.T) {
		td := Setup(t)

		a := createARecord(td)
		WaitForSync(td, a)

		err := td.Kube.Delete(td.Ctx, a)
		td.Require.NoError(err, "delete A record")
		WaitForDelete(td, a)
	})
}

func TestCNAMERecord(t *testing.T) {
	createCNAMERecord := func(td TestData) *endpoint.DNSEndpoint {
		td.T.Helper()

		cname := cnameRecord.DeepCopy()
		err := td.Kube.Create(td.Ctx, cname)
		td.Require.NoError(err, "create CNAME record")

		return cname
	}

	t.Run("create", func(t *testing.T) {
		td := Setup(t)

		cname := createCNAMERecord(td)
		WaitForSync(td, cname)
	})

	t.Run("update", func(t *testing.T) {
		td := Setup(t)

		cname := createCNAMERecord(td)
		WaitForSync(td, cname)

		cname.Spec.Endpoints[0].Targets[0] = "some-other-alias.testing"
		cname.Spec.Endpoints[0].RecordTTL = endpoint.TTL(60 * 60 * 4)
		err := td.Kube.Update(td.Ctx, cname)
		td.Require.NoError(err, "update CNAME record")
		WaitForSync(td, cname)
	})

	t.Run("delete", func(t *testing.T) {
		td := Setup(t)

		cname := createCNAMERecord(td)
		WaitForSync(td, cname)

		err := td.Kube.Delete(td.Ctx, cname)
		td.Require.NoError(err, "delete CNAME record")
		WaitForDelete(td, cname)
	})
}

func TestMXRecord(t *testing.T) {
	createMXRecord := func(td TestData) *endpoint.DNSEndpoint {
		td.T.Helper()

		mx := mxRecord.DeepCopy()
		err := td.Kube.Create(td.Ctx, mx)
		td.Require.NoError(err, "create MX record")

		return mx
	}

	t.Run("create", func(t *testing.T) {
		td := Setup(t)

		mx := createMXRecord(td)
		WaitForSync(td, mx)
	})

	t.Run("update", func(t *testing.T) {
		td := Setup(t)

		mx := createMXRecord(td)
		WaitForSync(td, mx)

		mx.Spec.Endpoints[0].Targets[0] = "30 another-exchange.testing"
		mx.Spec.Endpoints[0].RecordTTL = endpoint.TTL(60 * 60 * 4)
		err := td.Kube.Update(td.Ctx, mx)
		td.Require.NoError(err, "update MX record")
		WaitForSync(td, mx)
	})

	t.Run("delete", func(t *testing.T) {
		td := Setup(t)

		mx := createMXRecord(td)
		WaitForSync(td, mx)

		err := td.Kube.Delete(td.Ctx, mx)
		td.Require.NoError(err, "delete MX record")
		WaitForDelete(td, mx)
	})
}

func TestNSRecord(t *testing.T) {
	createNSRecord := func(td TestData) *endpoint.DNSEndpoint {
		td.T.Helper()

		ns := nsRecord.DeepCopy()
		err := td.Kube.Create(td.Ctx, ns)
		td.Require.NoError(err, "create NS record")

		return ns
	}

	t.Run("create", func(t *testing.T) {
		td := Setup(t)

		ns := createNSRecord(td)
		WaitForSync(td, ns)
	})

	t.Run("update", func(t *testing.T) {
		td := Setup(t)

		ns := createNSRecord(td)
		WaitForSync(td, ns)

		ns.Spec.Endpoints[0].Targets[0] = "another-nameserver.testing"
		ns.Spec.Endpoints[0].RecordTTL = endpoint.TTL(60 * 60 * 4)
		err := td.Kube.Update(td.Ctx, ns)
		td.Require.NoError(err, "update NS record")
		WaitForSync(td, ns)
	})

	t.Run("delete", func(t *testing.T) {
		td := Setup(t)

		ns := createNSRecord(td)
		WaitForSync(td, ns)

		err := td.Kube.Delete(td.Ctx, ns)
		td.Require.NoError(err, "delete NS record")
		WaitForDelete(td, ns)
	})
}

func TestSRVRecord(t *testing.T) {
	createSRVRecord := func(td TestData) *endpoint.DNSEndpoint {
		td.T.Helper()

		srv := srvRecord.DeepCopy()
		err := td.Kube.Create(td.Ctx, srv)
		td.Require.NoError(err, "create SRV record")

		return srv
	}

	t.Run("create", func(t *testing.T) {
		td := Setup(t)

		srv := createSRVRecord(td)
		WaitForSync(td, srv)
	})

	t.Run("update", func(t *testing.T) {
		td := Setup(t)

		srv := createSRVRecord(td)
		WaitForSync(td, srv)

		srv.Spec.Endpoints[0].Targets[0] = "1 2 3 another.testing"
		srv.Spec.Endpoints[0].RecordTTL = endpoint.TTL(60 * 60 * 4)
		err := td.Kube.Update(td.Ctx, srv)
		td.Require.NoError(err, "update SRV record")
		WaitForSync(td, srv)
	})

	t.Run("delete", func(t *testing.T) {
		td := Setup(t)

		srv := createSRVRecord(td)
		WaitForSync(td, srv)

		err := td.Kube.Delete(td.Ctx, srv)
		td.Require.NoError(err, "delete SRV record")
		WaitForDelete(td, srv)
	})
}

func TestTXTRecord(t *testing.T) {
	createTXTRecord := func(td TestData) *endpoint.DNSEndpoint {
		td.T.Helper()

		txt := txtRecord.DeepCopy()
		err := td.Kube.Create(td.Ctx, txt)
		td.Require.NoError(err, "create TXT record")

		return txt
	}

	t.Run("create", func(t *testing.T) {
		td := Setup(t)

		txt := createTXTRecord(td)
		WaitForSync(td, txt)
	})

	t.Run("update", func(t *testing.T) {
		td := Setup(t)

		txt := createTXTRecord(td)
		WaitForSync(td, txt)

		txt.Spec.Endpoints[0].Targets[0] = "different text"
		txt.Spec.Endpoints[0].RecordTTL = endpoint.TTL(60 * 60 * 4)
		err := td.Kube.Update(td.Ctx, txt)
		td.Require.NoError(err, "update TXT record")
		WaitForSync(td, txt)
	})

	t.Run("delete", func(t *testing.T) {
		td := Setup(t)

		txt := createTXTRecord(td)
		WaitForSync(td, txt)

		err := td.Kube.Delete(td.Ctx, txt)
		td.Require.NoError(err, "delete TXT record")
		WaitForDelete(td, txt)
	})
}
