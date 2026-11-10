package internal

import (
	"os"
	"path/filepath"

	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/common"
	"github.com/qzytwway/mplatform/pkg/model"
	"github.com/qzytwway/mplatform/pkg/rbac"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog/v2"
)

var (
	mplatformUsername = os.Getenv("MPLATFORM_USERNAME")
	mplatformPassword = os.Getenv("MPLATFORM_PASSWORD")
)

func loadUser() error {
	if mplatformUsername != "" && mplatformPassword != "" {
		uc, err := model.CountUsers()
		if err == nil && uc == 0 {
			klog.Infof("Creating super user %s from environment variables", mplatformUsername)
			u := &model.User{
				Username: mplatformUsername,
				Password: mplatformPassword,
			}
			err := model.AddSuperUser(u)
			if err == nil {
				rbac.SyncNow <- struct{}{}
			} else {
				return err
			}
		}
	}

	return nil
}

func loadClusters() error {
	cc, err := model.CountClusters()
	if err != nil || cc > 0 {
		return err
	}
	kubeconfigpath := ""
	if home := homedir.HomeDir(); home != "" {
		kubeconfigpath = filepath.Join(home, ".kube", "config")
	}

	if envKubeconfig := os.Getenv("KUBECONFIG"); envKubeconfig != "" {
		kubeconfigpath = envKubeconfig
	}

	config, _ := os.ReadFile(kubeconfigpath)

	if len(config) == 0 {
		return nil
	}
	kubeconfig, err := clientcmd.Load(config)
	if err != nil {
		return err
	}

	klog.Infof("Importing clusters from kubeconfig: %s", kubeconfigpath)
	cluster.ImportClustersFromKubeconfig(kubeconfig, true)
	return nil
}

func LoadConfigFromEnv() {
	if !common.IsSectionManaged("superUser") {
		if err := loadUser(); err != nil {
			klog.Warningf("Failed to migrate env to db user: %v", err)
		}
	}
	if !common.IsSectionManaged("clusters") {
		if err := loadClusters(); err != nil {
			klog.Warningf("Failed to migrate env to db cluster: %v", err)
		}
	}
}
