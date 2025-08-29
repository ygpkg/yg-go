package licensetool

import (
	"context"
	"crypto/rsa"
	"fmt"
	"os"

	"github.com/ygpkg/yg-go/logs"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Environment 定义了与特定环境交互的接口
type Environment interface {
	GetUID(ctx context.Context) (string, error)
	GetRawLicense(ctx context.Context) (string, error)
	GetPublicKey(ctx context.Context) (*rsa.PublicKey, error)
}

// --- Kubernetes Environment 实现 ---

// KubernetesEnvironment 实现了在 K8s 环境中获取数据
type KubernetesEnvironment struct {
	LicensePath   string
	PublicKeyPath string
}

func (k *KubernetesEnvironment) GetUID(ctx context.Context) (string, error) {
	clusterConfig, err := rest.InClusterConfig()
	if err != nil {
		return "", fmt.Errorf("failed to create in-cluster config: %w", err)
	}
	client, err := kubernetes.NewForConfig(clusterConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create Kubernetes client: %w", err)
	}
	namespace, err := client.CoreV1().Namespaces().Get(ctx, "kube-system", metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get kube-system namespace: %w", err)
	}
	return string(namespace.UID), nil
}

func (k *KubernetesEnvironment) GetRawLicense(ctx context.Context) (string, error) {
	bts, err := os.ReadFile(k.LicensePath)
	if err != nil {
		logs.ErrorContextf(ctx, "Failed to read license file %s: %v", k.LicensePath, err)
		return "", fmt.Errorf("failed to read license file %s: %w", k.LicensePath, err)
	}
	return string(bts), nil
}

func (k *KubernetesEnvironment) GetPublicKey(ctx context.Context) (*rsa.PublicKey, error) {
	bts, err := os.ReadFile(k.PublicKeyPath)
	if err != nil {
		logs.ErrorContextf(ctx, "Failed to read public key file %s: %v", k.PublicKeyPath, err)
		return nil, fmt.Errorf("failed to read public key file %s: %w", k.PublicKeyPath, err)
	}
	return ParsePublicKey(string(bts))
}

// NewEnvironment 根据环境类型创建对应的 Environment 实例
func NewEnvironment(envType EnvType) (Environment, error) {
	switch envType {
	case EnvTypeKubernetes:
		return &KubernetesEnvironment{
			LicensePath:   "/etc/sys/license/license.dat",
			PublicKeyPath: "/etc/sys/license/public.pem",
		}, nil
	default:
		return nil, fmt.Errorf("unknown environment type: %s", envType)
	}
}
