package k3sutils

import (
	"context"
	"time"

	"github.com/ipfs/go-log/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func init() {
	log.SetAllLoggers(log.LevelInfo)
	log.SetLogLevel("nimp2p-lab:k3s", "info")
	if cfg, err := rest.InClusterConfig(); err == nil {
		if K3sClient, err = kubernetes.NewForConfig(cfg); err == nil {
			return
		}
	}

	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	)
	cfg, err := kubeconfig.ClientConfig()
	if err != nil {
		logger.Fatalf("failed to access Kubeconfig: %v", err)
	}

	if K3sClient, err = kubernetes.NewForConfig(cfg); err != nil {
		logger.Fatalf("failed to create Client Config: %v", err)
	}

	// Ensure the default namespace exists --- IGNORE ---
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	_, err = K3sClient.CoreV1().Namespaces().Get(ctx, DEFAULT_NAMESPACE, metav1.GetOptions{})
	if err != nil {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: DEFAULT_NAMESPACE,
			},
		}
		if _, err := K3sClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{}); err != nil {
			logger.Fatalf("failed to create default namespace: %v", err)
		}
		logger.Infof("created default namespace '%s'", DEFAULT_NAMESPACE)
	} else {
		//logger.Infof("default namespace '%s' already exists", DEFAULT_NAMESPACE)
	}
}
