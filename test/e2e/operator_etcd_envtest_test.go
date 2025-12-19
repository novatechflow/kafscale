//go:build e2e

package e2e

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	kafscalev1alpha1 "github.com/novatechflow/kafscale/api/v1alpha1"
	"github.com/novatechflow/kafscale/pkg/operator"
)

func TestOperatorManagedEtcdResources(t *testing.T) {
	if !parseBoolEnv("KAFSCALE_E2E") {
		t.Skip("set KAFSCALE_E2E=1 to run operator envtest")
	}

	t.Setenv("KAFSCALE_OPERATOR_ETCD_ENDPOINTS", "")
	t.Setenv("KAFSCALE_OPERATOR_ETCD_SNAPSHOT_SKIP_PREFLIGHT", "1")

	crdPath := filepath.Join(repoRoot(t), "deploy", "helm", "kafscale", "crds")
	env := &envtest.Environment{
		CRDDirectoryPaths: []string{crdPath},
	}
	cfg, err := env.Start()
	if err != nil {
		t.Fatalf("start envtest: %v", err)
	}
	t.Cleanup(func() {
		if err := env.Stop(); err != nil {
			t.Fatalf("stop envtest: %v", err)
		}
	})

	scheme := k8sruntime.NewScheme()
	utilruntime.Must(kafscalev1alpha1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(appsv1.AddToScheme(scheme))
	utilruntime.Must(batchv1.AddToScheme(scheme))
	utilruntime.Must(policyv1.AddToScheme(scheme))

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	publisher := operator.NewSnapshotPublisher(mgr.GetClient())
	if err := operator.NewClusterReconciler(mgr, publisher).SetupWithManager(mgr); err != nil {
		t.Fatalf("cluster reconciler: %v", err)
	}
	if err := operator.NewTopicReconciler(mgr, publisher).SetupWithManager(mgr); err != nil {
		t.Fatalf("topic reconciler: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		if err := mgr.Start(ctx); err != nil {
			t.Errorf("manager error: %v", err)
		}
	}()

	cluster := &kafscalev1alpha1.KafscaleCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "etcd-managed",
			Namespace: "default",
		},
		Spec: kafscalev1alpha1.KafscaleClusterSpec{
			Brokers: kafscalev1alpha1.BrokerSpec{},
			S3: kafscalev1alpha1.S3Spec{
				Bucket: "snapshots",
				Region: "us-east-1",
			},
			Etcd: kafscalev1alpha1.EtcdSpec{},
		},
	}
	if err := mgr.GetClient().Create(ctx, cluster); err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer waitCancel()
	if err := wait.PollUntilContextTimeout(waitCtx, 200*time.Millisecond, 10*time.Second, true, func(ctx context.Context) (bool, error) {
		if !resourceExists(ctx, mgr.GetClient(), &appsv1.StatefulSet{}, cluster.Namespace, cluster.Name+"-etcd") {
			return false, nil
		}
		if !resourceExists(ctx, mgr.GetClient(), &corev1.Service{}, cluster.Namespace, cluster.Name+"-etcd") {
			return false, nil
		}
		if !resourceExists(ctx, mgr.GetClient(), &corev1.Service{}, cluster.Namespace, cluster.Name+"-etcd-client") {
			return false, nil
		}
		if !resourceExists(ctx, mgr.GetClient(), &policyv1.PodDisruptionBudget{}, cluster.Namespace, cluster.Name+"-etcd") {
			return false, nil
		}
		if !resourceExists(ctx, mgr.GetClient(), &batchv1.CronJob{}, cluster.Namespace, cluster.Name+"-etcd-snapshot") {
			return false, nil
		}
		return true, nil
	}); err != nil {
		t.Fatalf("expected managed etcd resources: %v", err)
	}
}

func resourceExists(ctx context.Context, c client.Reader, obj client.Object, ns, name string) bool {
	err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, obj)
	return err == nil || !apierrors.IsNotFound(err)
}
