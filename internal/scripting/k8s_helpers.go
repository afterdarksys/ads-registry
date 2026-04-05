package scripting

import (
	"fmt"
	"os/exec"
	"strings"

	"go.starlark.net/starlark"
)

// K8sHelpers provides Kubernetes-specific helper functions
type K8sHelpers struct {
	composeConverter *ComposeConverter
	manifestGen      *ManifestGenerator
	policyGen        *PolicyGenerator
}

// NewK8sHelpers creates new K8s helpers
func NewK8sHelpers() *K8sHelpers {
	return &K8sHelpers{
		composeConverter: NewComposeConverter(),
		manifestGen:      NewManifestGenerator(),
		policyGen:        NewPolicyGenerator(),
	}
}

// Module returns the k8s Starlark module
func (k *K8sHelpers) Module() starlark.StringDict {
	return starlark.StringDict{
		// Manifest generation
		"deployment":          starlark.NewBuiltin("deployment", k.createDeployment),
		"service":             starlark.NewBuiltin("service", k.createService),
		"config_map":          starlark.NewBuiltin("config_map", k.createConfigMap),
		"secret":              starlark.NewBuiltin("secret", k.createSecret),
		"ingress":             starlark.NewBuiltin("ingress", k.createIngress),
		"persistent_volume":   starlark.NewBuiltin("persistent_volume", k.createPV),
		"persistent_volume_claim": starlark.NewBuiltin("persistent_volume_claim", k.createPVC),
		"stateful_set":        starlark.NewBuiltin("stateful_set", k.createStatefulSet),
		"daemon_set":          starlark.NewBuiltin("daemon_set", k.createDaemonSet),
		"job":                 starlark.NewBuiltin("job", k.createJob),
		"cron_job":            starlark.NewBuiltin("cron_job", k.createCronJob),

		// Policy generation
		"network_policy":      starlark.NewBuiltin("network_policy", k.createNetworkPolicy),
		"pod_security_policy": starlark.NewBuiltin("pod_security_policy", k.createPodSecurityPolicy),
		"resource_quota":      starlark.NewBuiltin("resource_quota", k.createResourceQuota),
		"limit_range":         starlark.NewBuiltin("limit_range", k.createLimitRange),

		// RBAC
		"service_account":     starlark.NewBuiltin("service_account", k.createServiceAccount),
		"role":                starlark.NewBuiltin("role", k.createRole),
		"cluster_role":        starlark.NewBuiltin("cluster_role", k.createClusterRole),
		"role_binding":        starlark.NewBuiltin("role_binding", k.createRoleBinding),
		"cluster_role_binding": starlark.NewBuiltin("cluster_role_binding", k.createClusterRoleBinding),

		// Registry-specific
		"image_pull_secret":   starlark.NewBuiltin("image_pull_secret", k.createImagePullSecret),
		"registry_secret":     starlark.NewBuiltin("registry_secret", k.createRegistrySecret),

		// Utilities
		"namespace":           starlark.NewBuiltin("namespace", k.createNamespace),
		"apply":               starlark.NewBuiltin("apply", k.applyManifest),
		"delete":              starlark.NewBuiltin("delete", k.deleteResource),
		"get":                 starlark.NewBuiltin("get", k.getResource),
	}
}

// ComposeModule returns the docker-compose migration module
func (k *K8sHelpers) ComposeModule() starlark.StringDict {
	return starlark.StringDict{
		"to_k8s":              starlark.NewBuiltin("to_k8s", k.composeToK8s),
		"to_k3s":              starlark.NewBuiltin("to_k3s", k.composeToK3s),
		"parse":               starlark.NewBuiltin("parse", k.parseCompose),
		"validate":            starlark.NewBuiltin("validate", k.validateCompose),
	}
}

// ============================================================================
// MANIFEST GENERATION FUNCTIONS
// ============================================================================

func (k *K8sHelpers) createDeployment(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name, image string
	var replicas int = 1
	var namespace string = "default"

	if err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"name", &name,
		"image", &image,
		"replicas?", &replicas,
		"namespace?", &namespace,
	); err != nil {
		return nil, err
	}

	manifest := k.manifestGen.GenerateDeployment(name, namespace, image, int32(replicas))
	return starlark.String(manifest), nil
}

func (k *K8sHelpers) createService(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	var namespace string = "default"
	var serviceType string = "ClusterIP"
	var port int = 80

	if err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"name", &name,
		"namespace?", &namespace,
		"type?", &serviceType,
		"port?", &port,
	); err != nil {
		return nil, err
	}

	manifest := k.manifestGen.GenerateService(name, namespace, serviceType, int32(port))
	return starlark.String(manifest), nil
}

func (k *K8sHelpers) createConfigMap(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	var namespace string = "default"
	var data starlark.Value

	if err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"name", &name,
		"namespace?", &namespace,
		"data", &data,
	); err != nil {
		return nil, err
	}

	// Convert Starlark dict to Go map
	dataMap := make(map[string]string)
	if dict, ok := data.(*starlark.Dict); ok {
		for _, item := range dict.Items() {
			key, _ := item[0].(starlark.String)
			value, _ := item[1].(starlark.String)
			dataMap[string(key)] = string(value)
		}
	}

	manifest := k.manifestGen.GenerateConfigMap(name, namespace, dataMap)
	return starlark.String(manifest), nil
}

func (k *K8sHelpers) createSecret(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	var namespace string = "default"
	var data starlark.Value

	if err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"name", &name,
		"namespace?", &namespace,
		"data", &data,
	); err != nil {
		return nil, err
	}

	// Convert Starlark dict to Go map
	dataMap := make(map[string]string)
	if dict, ok := data.(*starlark.Dict); ok {
		for _, item := range dict.Items() {
			key, _ := item[0].(starlark.String)
			value, _ := item[1].(starlark.String)
			dataMap[string(key)] = string(value)
		}
	}

	manifest := k.manifestGen.GenerateSecret(name, namespace, dataMap)
	return starlark.String(manifest), nil
}

func (k *K8sHelpers) createIngress(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return starlark.String(fmt.Sprintf("apiVersion: networking.k8s.io/v1\nkind: Ingress\nmetadata:\n  name: ingress-placeholder\n")), nil
}

func (k *K8sHelpers) createPV(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return starlark.String(fmt.Sprintf("apiVersion: v1\nkind: PersistentVolume\nmetadata:\n  name: pv-placeholder\n")), nil
}

func (k *K8sHelpers) createPVC(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return starlark.String(fmt.Sprintf("apiVersion: v1\nkind: PersistentVolumeClaim\nmetadata:\n  name: pvc-placeholder\n")), nil
}

func (k *K8sHelpers) createStatefulSet(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return starlark.String(fmt.Sprintf("apiVersion: apps/v1\nkind: StatefulSet\nmetadata:\n  name: statefulset-placeholder\n")), nil
}

func (k *K8sHelpers) createDaemonSet(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return starlark.String(fmt.Sprintf("apiVersion: apps/v1\nkind: DaemonSet\nmetadata:\n  name: daemonset-placeholder\n")), nil
}

func (k *K8sHelpers) createJob(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return starlark.String(fmt.Sprintf("apiVersion: batch/v1\nkind: Job\nmetadata:\n  name: job-placeholder\n")), nil
}

func (k *K8sHelpers) createCronJob(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return starlark.String(fmt.Sprintf("apiVersion: batch/v1\nkind: CronJob\nmetadata:\n  name: cronjob-placeholder\n")), nil
}

// ============================================================================
// POLICY GENERATION FUNCTIONS
// ============================================================================

func (k *K8sHelpers) createNetworkPolicy(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	var namespace string = "default"

	if err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"name", &name,
		"namespace?", &namespace,
	); err != nil {
		return nil, err
	}

	manifest := k.policyGen.GenerateNetworkPolicy(name, namespace)
	return starlark.String(manifest), nil
}

func (k *K8sHelpers) createPodSecurityPolicy(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string

	if err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"name", &name,
	); err != nil {
		return nil, err
	}

	manifest := k.policyGen.GeneratePodSecurityPolicy(name)
	return starlark.String(manifest), nil
}

func (k *K8sHelpers) createResourceQuota(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return starlark.String(fmt.Sprintf("apiVersion: v1\nkind: ResourceQuota\nmetadata:\n  name: quota-placeholder\n")), nil
}

func (k *K8sHelpers) createLimitRange(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return starlark.String(fmt.Sprintf("apiVersion: v1\nkind: LimitRange\nmetadata:\n  name: limitrange-placeholder\n")), nil
}

// ============================================================================
// RBAC FUNCTIONS
// ============================================================================

func (k *K8sHelpers) createServiceAccount(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	var namespace string = "default"

	if err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"name", &name,
		"namespace?", &namespace,
	); err != nil {
		return nil, err
	}

	manifest := k.manifestGen.GenerateServiceAccount(name, namespace)
	return starlark.String(manifest), nil
}

func (k *K8sHelpers) createRole(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return starlark.String(fmt.Sprintf("apiVersion: rbac.authorization.k8s.io/v1\nkind: Role\nmetadata:\n  name: role-placeholder\n")), nil
}

func (k *K8sHelpers) createClusterRole(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return starlark.String(fmt.Sprintf("apiVersion: rbac.authorization.k8s.io/v1\nkind: ClusterRole\nmetadata:\n  name: clusterrole-placeholder\n")), nil
}

func (k *K8sHelpers) createRoleBinding(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return starlark.String(fmt.Sprintf("apiVersion: rbac.authorization.k8s.io/v1\nkind: RoleBinding\nmetadata:\n  name: rolebinding-placeholder\n")), nil
}

func (k *K8sHelpers) createClusterRoleBinding(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return starlark.String(fmt.Sprintf("apiVersion: rbac.authorization.k8s.io/v1\nkind: ClusterRoleBinding\nmetadata:\n  name: clusterrolebinding-placeholder\n")), nil
}

// ============================================================================
// REGISTRY-SPECIFIC FUNCTIONS
// ============================================================================

func (k *K8sHelpers) createImagePullSecret(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name, registry, username, password string
	var namespace string = "default"

	if err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"name", &name,
		"registry", &registry,
		"username", &username,
		"password", &password,
		"namespace?", &namespace,
	); err != nil {
		return nil, err
	}

	manifest := k.manifestGen.GenerateImagePullSecret(name, namespace, registry, username, password)
	return starlark.String(manifest), nil
}

func (k *K8sHelpers) createRegistrySecret(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	// Alias for image_pull_secret
	return k.createImagePullSecret(thread, fn, args, kwargs)
}

// ============================================================================
// DOCKER COMPOSE MIGRATION FUNCTIONS
// ============================================================================

func (k *K8sHelpers) composeToK8s(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var composePath string
	var outputPath string = "k8s-manifests"

	if err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"compose_file", &composePath,
		"output?", &outputPath,
	); err != nil {
		return nil, err
	}

	manifests, err := k.composeConverter.ConvertToK8s(composePath, false)
	if err != nil {
		return nil, err
	}

	return starlark.String(manifests), nil
}

func (k *K8sHelpers) composeToK3s(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var composePath string
	var outputPath string = "k3s-manifests"

	if err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"compose_file", &composePath,
		"output?", &outputPath,
	); err != nil {
		return nil, err
	}

	manifests, err := k.composeConverter.ConvertToK8s(composePath, true)
	if err != nil {
		return nil, err
	}

	return starlark.String(manifests), nil
}

func (k *K8sHelpers) parseCompose(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	// TODO: Implement
	return starlark.None, nil
}

func (k *K8sHelpers) validateCompose(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	// TODO: Implement
	return starlark.True, nil
}

// ============================================================================
// UTILITY FUNCTIONS
// ============================================================================

func (k *K8sHelpers) createNamespace(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string

	if err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"name", &name,
	); err != nil {
		return nil, err
	}

	manifest := k.manifestGen.GenerateNamespace(name)
	return starlark.String(manifest), nil
}

func (k *K8sHelpers) applyManifest(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var manifest string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "manifest", &manifest); err != nil {
		return nil, err
	}
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("kubectl apply failed: %v output: %s", err, string(out))
	}
	return starlark.String(string(out)), nil
}

func (k *K8sHelpers) deleteResource(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var manifest string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "manifest", &manifest); err != nil {
		return nil, err
	}
	cmd := exec.Command("kubectl", "delete", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("kubectl delete failed: %v output: %s", err, string(out))
	}
	return starlark.String(string(out)), nil
}

func (k *K8sHelpers) getResource(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var resourceType, resourceName, namespace string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "type", &resourceType, "name", &resourceName, "namespace?", &namespace); err != nil {
		return nil, err
	}
	
	cmdArgs := []string{"get", resourceType, resourceName, "-o", "json"}
	if namespace != "" {
		cmdArgs = append(cmdArgs, "-n", namespace)
	}

	cmd := exec.Command("kubectl", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("kubectl get failed: %v output: %s", err, string(out))
	}
	return starlark.String(string(out)), nil
}
