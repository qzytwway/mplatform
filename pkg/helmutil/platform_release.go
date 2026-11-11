package helmutil

import (
	"context"
	"errors"
	"fmt"

	"helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	release "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage/driver"
	"k8s.io/klog/v2"
)

// The functions in this file manage releases the platform installs itself
// from the local chart directory, such as the logging stack.

var ErrReleaseAlreadyInstalled = errors.New("already installed")

// withoutValues is the release as the history records it: the platform sets
// the values of these releases itself, and they may hold secrets.
func withoutValues(rel *release.Release) *release.Release {
	if rel == nil {
		return nil
	}
	recorded := *rel
	recorded.Config = nil
	return &recorded
}

func ReleaseExists(cfg *action.Configuration, name string) (bool, error) {
	if _, err := GetRelease(cfg, name); err != nil {
		if errors.Is(err, driver.ErrReleaseNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ClaimReleaseName fails with ErrReleaseAlreadyInstalled when the release
// exists, and removes a failed earlier attempt that still holds the name.
func ClaimReleaseName(cfg *action.Configuration, clusterName string, operatorID uint, name, namespace string, opts UninstallReleaseOptions) error {
	existing, err := GetRelease(cfg, name)
	switch {
	case errors.Is(err, driver.ErrReleaseNotFound):
		return nil
	case err != nil:
		return err
	case existing.Info == nil || existing.Info.Status != releasecommon.StatusFailed:
		return fmt.Errorf("%s is %w", name, ErrReleaseAlreadyInstalled)
	}
	return UninstallPlatformRelease(cfg, clusterName, operatorID, name, namespace, opts)
}

// InstallPlatformRelease installs a chart with the cluster's image registry
// and records it in the release history. A failed release is removed so the
// install can be retried.
func InstallPlatformRelease(ctx context.Context, cfg *action.Configuration, clusterName string, operatorID uint, loadedChart *chart.Chart, values map[string]any, opts InstallReleaseOptions) (err error) {
	imageRegistry, err := ClusterImageRegistry(clusterName)
	if err != nil {
		return err
	}
	installOpts := opts
	installOpts.ImageRegistry = imageRegistry

	var rel *release.Release
	defer func() {
		RecordReleaseHistory(clusterName, operatorID, "manual", "install", opts.ReleaseName, opts.Namespace, nil, withoutValues(rel), err == nil, err)
	}()
	rel, err = InstallRelease(ctx, cfg, loadedChart, values, installOpts)
	if err != nil {
		if _, getErr := GetRelease(cfg, opts.ReleaseName); getErr == nil {
			if uninstallErr := UninstallRelease(cfg, opts.ReleaseName, UninstallReleaseOptions{Timeout: opts.Timeout}); uninstallErr != nil {
				klog.Warningf("Failed to remove failed release %s in cluster %s: %v", opts.ReleaseName, clusterName, uninstallErr)
			}
		}
	}
	return err
}

// UpgradePlatformRelease upgrades a release to a chart and values with the
// cluster's image registry, and records it in the release history.
func UpgradePlatformRelease(ctx context.Context, cfg *action.Configuration, clusterName string, operatorID uint, name string, loadedChart *chart.Chart, values map[string]any, opts UpgradeReleaseOptions) (err error) {
	imageRegistry, err := ClusterImageRegistry(clusterName)
	if err != nil {
		return err
	}
	current, err := GetRelease(cfg, name)
	if err != nil {
		return err
	}
	upgradeOpts := opts
	upgradeOpts.ImageRegistry = imageRegistry

	var rel *release.Release
	defer func() {
		RecordReleaseHistory(clusterName, operatorID, "manual", "upgrade", name, opts.Namespace, withoutValues(current), withoutValues(rel), err == nil, err)
	}()
	rel, err = UpgradeRelease(ctx, cfg, name, loadedChart, values, upgradeOpts)
	return err
}

// UninstallPlatformRelease uninstalls a release and records it in the release
// history.
func UninstallPlatformRelease(cfg *action.Configuration, clusterName string, operatorID uint, name, namespace string, opts UninstallReleaseOptions) (err error) {
	current, err := GetRelease(cfg, name)
	if err != nil {
		return err
	}
	defer func() {
		RecordReleaseHistory(clusterName, operatorID, "manual", "delete", name, namespace, withoutValues(current), nil, err == nil, err)
	}()
	return UninstallRelease(cfg, name, opts)
}
