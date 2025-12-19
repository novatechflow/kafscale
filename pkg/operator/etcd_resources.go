package operator

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	kafscalev1alpha1 "github.com/novatechflow/kafscale/api/v1alpha1"
)

const (
	operatorEtcdEndpointsEnv             = "KAFSCALE_OPERATOR_ETCD_ENDPOINTS"
	operatorEtcdImageEnv                 = "KAFSCALE_OPERATOR_ETCD_IMAGE"
	operatorEtcdStorageEnv               = "KAFSCALE_OPERATOR_ETCD_STORAGE_SIZE"
	operatorEtcdClassEnv                 = "KAFSCALE_OPERATOR_ETCD_STORAGE_CLASS"
	operatorEtcdSnapshotBucketEnv        = "KAFSCALE_OPERATOR_ETCD_SNAPSHOT_BUCKET"
	operatorEtcdSnapshotPrefixEnv        = "KAFSCALE_OPERATOR_ETCD_SNAPSHOT_PREFIX"
	operatorEtcdSnapshotScheduleEnv      = "KAFSCALE_OPERATOR_ETCD_SNAPSHOT_SCHEDULE"
	operatorEtcdSnapshotImageEnv         = "KAFSCALE_OPERATOR_ETCD_SNAPSHOT_IMAGE"
	operatorEtcdSnapshotEtcdctlEnv       = "KAFSCALE_OPERATOR_ETCD_SNAPSHOT_ETCDCTL_IMAGE"
	operatorEtcdSnapshotEndpointEnv      = "KAFSCALE_OPERATOR_ETCD_SNAPSHOT_S3_ENDPOINT"
	operatorEtcdSnapshotStaleAfterEnv    = "KAFSCALE_OPERATOR_ETCD_SNAPSHOT_STALE_AFTER_SEC"
	operatorEtcdSnapshotCreateBucketEnv  = "KAFSCALE_OPERATOR_ETCD_SNAPSHOT_CREATE_BUCKET"
	operatorEtcdSnapshotProtectBucketEnv = "KAFSCALE_OPERATOR_ETCD_SNAPSHOT_PROTECT_BUCKET"

	defaultEtcdImage                 = "quay.io/coreos/etcd:v3.5.12"
	defaultEtcdStorageSize           = "10Gi"
	defaultSnapshotPrefix            = "etcd-snapshots"
	defaultSnapshotSchedule          = "0 * * * *"
	defaultSnapshotImage             = "amazon/aws-cli:2.15.0"
	defaultSnapshotStaleAfterSeconds = 2 * 60 * 60
)

type EtcdResolution struct {
	Endpoints []string
	Managed   bool
}

func EnsureEtcd(ctx context.Context, c client.Client, scheme *runtime.Scheme, cluster *kafscalev1alpha1.KafscaleCluster) (EtcdResolution, error) {
	if endpoints := cleanEndpoints(cluster.Spec.Etcd.Endpoints); len(endpoints) > 0 {
		return EtcdResolution{Endpoints: endpoints}, nil
	}
	if envEndpoints := parseEnvEndpoints(operatorEtcdEndpointsEnv); len(envEndpoints) > 0 {
		return EtcdResolution{Endpoints: envEndpoints}, nil
	}

	if err := reconcileEtcdResources(ctx, c, scheme, cluster); err != nil {
		return EtcdResolution{}, err
	}
	return EtcdResolution{Endpoints: managedEtcdEndpoints(cluster), Managed: true}, nil
}

func parseEnvEndpoints(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	return cleanEndpoints(strings.Split(raw, ","))
}

func cleanEndpoints(list []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(list))
	for _, entry := range list {
		val := strings.TrimSpace(entry)
		if val == "" {
			continue
		}
		if _, ok := seen[val]; ok {
			continue
		}
		seen[val] = struct{}{}
		out = append(out, val)
	}
	return out
}

func managedEtcdEndpoints(cluster *kafscalev1alpha1.KafscaleCluster) []string {
	host := fmt.Sprintf("%s-etcd-client.%s.svc.cluster.local:2379", cluster.Name, cluster.Namespace)
	return []string{"http://" + host}
}

func reconcileEtcdResources(ctx context.Context, c client.Client, scheme *runtime.Scheme, cluster *kafscalev1alpha1.KafscaleCluster) error {
	if err := reconcileEtcdHeadlessService(ctx, c, scheme, cluster); err != nil {
		return err
	}
	if err := reconcileEtcdClientService(ctx, c, scheme, cluster); err != nil {
		return err
	}
	if err := reconcileEtcdStatefulSet(ctx, c, scheme, cluster); err != nil {
		return err
	}
	if err := reconcileEtcdPDB(ctx, c, scheme, cluster); err != nil {
		return err
	}
	if err := reconcileEtcdSnapshotCronJob(ctx, c, scheme, cluster); err != nil {
		return err
	}
	return nil
}

func reconcileEtcdHeadlessService(ctx context.Context, c client.Client, scheme *runtime.Scheme, cluster *kafscalev1alpha1.KafscaleCluster) error {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name:      fmt.Sprintf("%s-etcd", cluster.Name),
		Namespace: cluster.Namespace,
	}}
	_, err := controllerutil.CreateOrUpdate(ctx, c, svc, func() error {
		labels := etcdLabels(cluster)
		svc.Labels = labels
		svc.Spec.ClusterIP = corev1.ClusterIPNone
		svc.Spec.Selector = labels
		svc.Spec.Ports = []corev1.ServicePort{
			{Name: "client", Port: 2379, TargetPort: intstr.FromInt(2379)},
			{Name: "peer", Port: 2380, TargetPort: intstr.FromInt(2380)},
		}
		return controllerutil.SetControllerReference(cluster, svc, scheme)
	})
	return err
}

func reconcileEtcdClientService(ctx context.Context, c client.Client, scheme *runtime.Scheme, cluster *kafscalev1alpha1.KafscaleCluster) error {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name:      fmt.Sprintf("%s-etcd-client", cluster.Name),
		Namespace: cluster.Namespace,
	}}
	_, err := controllerutil.CreateOrUpdate(ctx, c, svc, func() error {
		labels := etcdLabels(cluster)
		svc.Labels = labels
		svc.Spec.Selector = labels
		svc.Spec.Ports = []corev1.ServicePort{
			{Name: "client", Port: 2379, TargetPort: intstr.FromInt(2379)},
		}
		return controllerutil.SetControllerReference(cluster, svc, scheme)
	})
	return err
}

func reconcileEtcdStatefulSet(ctx context.Context, c client.Client, scheme *runtime.Scheme, cluster *kafscalev1alpha1.KafscaleCluster) error {
	sts := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{
		Name:      fmt.Sprintf("%s-etcd", cluster.Name),
		Namespace: cluster.Namespace,
	}}
	_, err := controllerutil.CreateOrUpdate(ctx, c, sts, func() error {
		labels := etcdLabels(cluster)
		replicas := int32(3)
		sts.Labels = labels
		sts.Spec.ServiceName = fmt.Sprintf("%s-etcd", cluster.Name)
		sts.Spec.Replicas = &replicas
		sts.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
		sts.Spec.Template.ObjectMeta.Labels = labels

		storageSize := getEnv(operatorEtcdStorageEnv, defaultEtcdStorageSize)
		storageClass := strings.TrimSpace(os.Getenv(operatorEtcdClassEnv))
		sts.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "data"},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse(storageSize),
						},
					},
					StorageClassName: stringPtrOrNil(storageClass),
				},
			},
		}

		image := getEnv(operatorEtcdImageEnv, defaultEtcdImage)
		sts.Spec.Template.Spec.Containers = []corev1.Container{
			{
				Name:  "etcd",
				Image: image,
				Ports: []corev1.ContainerPort{
					{Name: "client", ContainerPort: 2379},
					{Name: "peer", ContainerPort: 2380},
				},
				Env: []corev1.EnvVar{
					{Name: "POD_NAME", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}}},
					{Name: "POD_NAMESPACE", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"}}},
				},
				Command: []string{"etcd"},
				Args:    etcdArgs(cluster),
				VolumeMounts: []corev1.VolumeMount{
					{Name: "data", MountPath: "/var/lib/etcd"},
				},
			},
		}
		return controllerutil.SetControllerReference(cluster, sts, scheme)
	})
	return err
}

func etcdArgs(cluster *kafscalev1alpha1.KafscaleCluster) []string {
	peerSvc := fmt.Sprintf("%s-etcd", cluster.Name)
	initialCluster := fmt.Sprintf(
		"%s-etcd-0=http://%s-etcd-0.%s.$(POD_NAMESPACE).svc.cluster.local:2380,%s-etcd-1=http://%s-etcd-1.%s.$(POD_NAMESPACE).svc.cluster.local:2380,%s-etcd-2=http://%s-etcd-2.%s.$(POD_NAMESPACE).svc.cluster.local:2380",
		cluster.Name, cluster.Name, peerSvc,
		cluster.Name, cluster.Name, peerSvc,
		cluster.Name, cluster.Name, peerSvc,
	)
	peerURL := fmt.Sprintf("http://$(POD_NAME).%s.$(POD_NAMESPACE).svc.cluster.local:2380", peerSvc)
	clientURL := fmt.Sprintf("http://$(POD_NAME).%s.$(POD_NAMESPACE).svc.cluster.local:2379", peerSvc)
	return []string{
		"--name=$(POD_NAME)",
		"--data-dir=/var/lib/etcd",
		"--listen-peer-urls=http://0.0.0.0:2380",
		"--listen-client-urls=http://0.0.0.0:2379",
		"--advertise-client-urls=" + clientURL,
		"--initial-advertise-peer-urls=" + peerURL,
		"--initial-cluster=" + initialCluster,
		"--initial-cluster-state=new",
		"--initial-cluster-token=" + cluster.Name + "-etcd",
	}
}

func reconcileEtcdPDB(ctx context.Context, c client.Client, scheme *runtime.Scheme, cluster *kafscalev1alpha1.KafscaleCluster) error {
	pdb := &policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{
		Name:      fmt.Sprintf("%s-etcd", cluster.Name),
		Namespace: cluster.Namespace,
	}}
	_, err := controllerutil.CreateOrUpdate(ctx, c, pdb, func() error {
		labels := etcdLabels(cluster)
		pdb.Labels = labels
		pdb.Spec = policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: intstrPtr(1),
			Selector:       &metav1.LabelSelector{MatchLabels: labels},
		}
		return controllerutil.SetControllerReference(cluster, pdb, scheme)
	})
	return err
}

func reconcileEtcdSnapshotCronJob(ctx context.Context, c client.Client, scheme *runtime.Scheme, cluster *kafscalev1alpha1.KafscaleCluster) error {
	bucket := strings.TrimSpace(os.Getenv(operatorEtcdSnapshotBucketEnv))
	if bucket == "" {
		bucket = strings.TrimSpace(cluster.Spec.S3.Bucket)
	}
	if bucket == "" {
		return nil
	}
	prefix := getEnv(operatorEtcdSnapshotPrefixEnv, defaultSnapshotPrefix)
	schedule := getEnv(operatorEtcdSnapshotScheduleEnv, defaultSnapshotSchedule)
	endpoint := strings.TrimSpace(os.Getenv(operatorEtcdSnapshotEndpointEnv))
	etcdctlImage := getEnv(operatorEtcdSnapshotEtcdctlEnv, defaultEtcdImage)
	backupImage := getEnv(operatorEtcdSnapshotImageEnv, defaultSnapshotImage)
	createBucket := parseBoolEnv(operatorEtcdSnapshotCreateBucketEnv)
	protectBucket := parseBoolEnv(operatorEtcdSnapshotProtectBucketEnv)

	cron := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{
		Name:      fmt.Sprintf("%s-etcd-snapshot", cluster.Name),
		Namespace: cluster.Namespace,
	}}

	_, err := controllerutil.CreateOrUpdate(ctx, c, cron, func() error {
		labels := etcdLabels(cluster)
		cron.Labels = labels
		cron.Spec.Schedule = schedule
		cron.Spec.ConcurrencyPolicy = batchv1.ForbidConcurrent
		cron.Spec.SuccessfulJobsHistoryLimit = int32Ptr(3)
		cron.Spec.FailedJobsHistoryLimit = int32Ptr(3)
		cron.Spec.JobTemplate.Spec.Template.ObjectMeta.Labels = labels
		cron.Spec.JobTemplate.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyNever
		cron.Spec.JobTemplate.Spec.Template.Spec.Volumes = []corev1.Volume{
			{
				Name: "snapshots",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		}

		envFrom := []corev1.EnvFromSource{}
		if strings.TrimSpace(cluster.Spec.S3.CredentialsSecretRef) != "" {
			envFrom = append(envFrom, corev1.EnvFromSource{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: cluster.Spec.S3.CredentialsSecretRef},
					Optional:             boolPtr(true),
				},
			})
		}

		cron.Spec.JobTemplate.Spec.Template.Spec.InitContainers = []corev1.Container{
			{
				Name:  "snapshot",
				Image: etcdctlImage,
				Command: []string{
					"/bin/sh",
					"-c",
					"ETCDCTL_API=3 etcdctl --endpoints=$ETCD_ENDPOINTS snapshot save /snapshots/etcd-snapshot.db",
				},
				Env: []corev1.EnvVar{
					{Name: "ETCD_ENDPOINTS", Value: strings.Join(managedEtcdEndpoints(cluster), ",")},
				},
				VolumeMounts: []corev1.VolumeMount{
					{Name: "snapshots", MountPath: "/snapshots"},
				},
			},
		}

		uploadEnv := []corev1.EnvVar{
			{Name: "AWS_REGION", Value: cluster.Spec.S3.Region},
			{Name: "SNAPSHOT_BUCKET", Value: bucket},
			{Name: "SNAPSHOT_PREFIX", Value: strings.Trim(prefix, "/")},
			{Name: "CREATE_BUCKET", Value: boolToString(createBucket)},
			{Name: "PROTECT_BUCKET", Value: boolToString(protectBucket)},
		}
		if endpoint != "" {
			uploadEnv = append(uploadEnv, corev1.EnvVar{Name: "AWS_ENDPOINT_URL", Value: endpoint})
		}

		cron.Spec.JobTemplate.Spec.Template.Spec.Containers = []corev1.Container{
			{
				Name:  "upload",
				Image: backupImage,
				Command: []string{
					"/bin/sh",
					"-c",
					"set -euo pipefail\n" +
						"TS=$(date -u +%Y%m%d%H%M%S)\n" +
						"SNAPSHOT=/snapshots/etcd-snapshot.db\n" +
						"CHECKSUM=/snapshots/etcd-snapshot.db.sha256\n" +
						"if [ \"$CREATE_BUCKET\" = \"1\" ]; then\n" +
						"  if ! aws s3api head-bucket --bucket \"$SNAPSHOT_BUCKET\" >/dev/null 2>&1; then\n" +
						"    if [ \"$AWS_REGION\" = \"us-east-1\" ]; then\n" +
						"      aws s3api create-bucket --bucket \"$SNAPSHOT_BUCKET\"\n" +
						"    else\n" +
						"      aws s3api create-bucket --bucket \"$SNAPSHOT_BUCKET\" --create-bucket-configuration LocationConstraint=\"$AWS_REGION\"\n" +
						"    fi\n" +
						"  fi\n" +
						"fi\n" +
						"if [ \"$PROTECT_BUCKET\" = \"1\" ]; then\n" +
						"  aws s3api head-bucket --bucket \"$SNAPSHOT_BUCKET\" >/dev/null\n" +
						"  aws s3api put-bucket-versioning --bucket \"$SNAPSHOT_BUCKET\" --versioning-configuration Status=Enabled\n" +
						"  aws s3api put-public-access-block --bucket \"$SNAPSHOT_BUCKET\" --public-access-block-configuration BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true\n" +
						"fi\n" +
						"sha256sum \"$SNAPSHOT\" > \"$CHECKSUM\"\n" +
						"aws s3 cp \"$SNAPSHOT\" \"s3://$SNAPSHOT_BUCKET/$SNAPSHOT_PREFIX/$TS.db\"\n" +
						"aws s3 cp \"$CHECKSUM\" \"s3://$SNAPSHOT_BUCKET/$SNAPSHOT_PREFIX/$TS.db.sha256\"",
				},
				Env:     uploadEnv,
				EnvFrom: envFrom,
				VolumeMounts: []corev1.VolumeMount{
					{Name: "snapshots", MountPath: "/snapshots"},
				},
			},
		}
		return controllerutil.SetControllerReference(cluster, cron, scheme)
	})
	return err
}

func etcdLabels(cluster *kafscalev1alpha1.KafscaleCluster) map[string]string {
	return map[string]string{
		"app":     "kafscale-etcd",
		"cluster": cluster.Name,
	}
}

func intstrPtr(val int) *intstr.IntOrString {
	v := intstr.FromInt(val)
	return &v
}

func snapshotStaleAfterSeconds() int64 {
	val := strings.TrimSpace(os.Getenv(operatorEtcdSnapshotStaleAfterEnv))
	if val == "" {
		return defaultSnapshotStaleAfterSeconds
	}
	parsed, err := strconv.ParseInt(val, 10, 64)
	if err != nil || parsed <= 0 {
		return defaultSnapshotStaleAfterSeconds
	}
	return parsed
}

func parseBoolEnv(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func boolToString(val bool) string {
	if val {
		return "1"
	}
	return "0"
}

func stringPtrOrNil(val string) *string {
	if strings.TrimSpace(val) == "" {
		return nil
	}
	return &val
}
