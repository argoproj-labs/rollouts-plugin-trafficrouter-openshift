package fixtures

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	rolloutsclient "github.com/argoproj/argo-rollouts/pkg/client/clientset/versioned"
	openshiftclientset "github.com/openshift/client-go/route/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	RolloutsE2ENamespace = "argo-rollouts-e2e"
)

// EnsureCleanState ensures that a clean namespace is available to run e2e tests.
func EnsureCleanState() error {
	ctx := context.Background()
	k8sClient, err := GetK8sClient()
	if err != nil {
		return err
	}

	if err = deleteNamespace(ctx, RolloutsE2ENamespace, k8sClient); err != nil {
		return err
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: RolloutsE2ENamespace,
		},
	}
	_, err = k8sClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	return err
}

func deleteNamespace(ctx context.Context, name string, k8sClient kubernetes.Interface) error {
	return wait.PollImmediate(3*time.Second, 5*time.Minute, func() (done bool, err error) {
		err = k8sClient.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			fmt.Printf("failed to delete namespace %s: %v\n", name, err)
			return false, nil
		}

		_, err = k8sClient.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			fmt.Printf("failed to get namespace %s: %v\n", name, err)
			return false, nil
		}
		return false, nil
	})
}

func GetK8sClient() (*kubernetes.Clientset, error) {
	restConfig, err := getK8sConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(restConfig)
}

func GetRolloutsClient() (rolloutsclient.Interface, error) {
	restConfig, err := getK8sConfig()
	if err != nil {
		return nil, err
	}
	return rolloutsclient.NewForConfig(restConfig)
}

func GetRouteClient() (openshiftclientset.Interface, error) {
	restConfig, err := getK8sConfig()
	if err != nil {
		return nil, err
	}
	return openshiftclientset.NewForConfig(restConfig)
}

func ApplyResources(path, ns string) error {
	cmd := exec.Command("kubectl", "apply", "-f", path, "-n", ns)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	fmt.Println(string(out))

	return err
}

func getK8sConfig() (*rest.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	config := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
	return config.ClientConfig()
}
