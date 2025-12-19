package operator

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	kafscalev1alpha1 "github.com/novatechflow/kafscale/api/v1alpha1"
)

func TestEnsureEtcdUsesSpecEndpoints(t *testing.T) {
	t.Setenv(operatorEtcdEndpointsEnv, "http://env-etcd:2379")
	cluster := testCluster("spec-endpoints", []string{"http://spec-etcd:2379"})
	scheme := testScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()

	res, err := EnsureEtcd(context.Background(), c, scheme, cluster)
	if err != nil {
		t.Fatalf("EnsureEtcd: %v", err)
	}
	if res.Managed {
		t.Fatalf("expected unmanaged etcd resolution")
	}
	if len(res.Endpoints) != 1 || res.Endpoints[0] != "http://spec-etcd:2379" {
		t.Fatalf("unexpected endpoints: %v", res.Endpoints)
	}

	assertNotFound(t, c, &appsv1.StatefulSet{}, cluster.Namespace, cluster.Name+"-etcd")
}

func TestEnsureEtcdUsesEnvEndpoints(t *testing.T) {
	t.Setenv(operatorEtcdEndpointsEnv, "http://env-etcd:2379")
	cluster := testCluster("env-endpoints", nil)
	scheme := testScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()

	res, err := EnsureEtcd(context.Background(), c, scheme, cluster)
	if err != nil {
		t.Fatalf("EnsureEtcd: %v", err)
	}
	if res.Managed {
		t.Fatalf("expected unmanaged etcd resolution")
	}
	if len(res.Endpoints) != 1 || res.Endpoints[0] != "http://env-etcd:2379" {
		t.Fatalf("unexpected endpoints: %v", res.Endpoints)
	}

	assertNotFound(t, c, &appsv1.StatefulSet{}, cluster.Namespace, cluster.Name+"-etcd")
}

func TestEnsureEtcdCreatesManagedCluster(t *testing.T) {
	t.Setenv(operatorEtcdEndpointsEnv, "")
	cluster := testCluster("managed", nil)
	scheme := testScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()

	res, err := EnsureEtcd(context.Background(), c, scheme, cluster)
	if err != nil {
		t.Fatalf("EnsureEtcd: %v", err)
	}
	if !res.Managed {
		t.Fatalf("expected managed etcd resolution")
	}
	if len(res.Endpoints) != 1 {
		t.Fatalf("unexpected endpoints: %v", res.Endpoints)
	}

	assertFound(t, c, &appsv1.StatefulSet{}, cluster.Namespace, cluster.Name+"-etcd")
	assertFound(t, c, &corev1.Service{}, cluster.Namespace, cluster.Name+"-etcd")
	assertFound(t, c, &corev1.Service{}, cluster.Namespace, cluster.Name+"-etcd-client")
	assertFound(t, c, &policyv1.PodDisruptionBudget{}, cluster.Namespace, cluster.Name+"-etcd")
	assertFound(t, c, &batchv1.CronJob{}, cluster.Namespace, cluster.Name+"-etcd-snapshot")
}

func testCluster(name string, endpoints []string) *kafscalev1alpha1.KafscaleCluster {
	return &kafscalev1alpha1.KafscaleCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: kafscalev1alpha1.KafscaleClusterSpec{
			Brokers: kafscalev1alpha1.BrokerSpec{},
			S3: kafscalev1alpha1.S3Spec{
				Bucket: "bucket",
				Region: "us-east-1",
			},
			Etcd: kafscalev1alpha1.EtcdSpec{Endpoints: endpoints},
		},
	}
}

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := kafscalev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add kafscale scheme: %v", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add apps scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := policyv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add policy scheme: %v", err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add batch scheme: %v", err)
	}
	return scheme
}

func assertFound(t *testing.T, c client.Client, obj client.Object, ns, name string) {
	t.Helper()
	key := client.ObjectKey{Namespace: ns, Name: name}
	if err := c.Get(context.Background(), key, obj); err != nil {
		t.Fatalf("expected %T %s/%s to exist: %v", obj, ns, name, err)
	}
}

func assertNotFound(t *testing.T, c client.Client, obj client.Object, ns, name string) {
	t.Helper()
	key := client.ObjectKey{Namespace: ns, Name: name}
	if err := c.Get(context.Background(), key, obj); err == nil {
		t.Fatalf("expected %T %s/%s to be absent", obj, ns, name)
	}
}
