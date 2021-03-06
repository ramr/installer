package manifests

import (
	"os"
	"path/filepath"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"

	"github.com/openshift/installer/pkg/asset"
	"github.com/openshift/installer/pkg/asset/installconfig"

	netopv1 "github.com/openshift/cluster-network-operator/pkg/apis/networkoperator/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1a1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

var (
	noCrdFilename = filepath.Join(manifestDir, "cluster-network-01-crd.yml")
	noCfgFilename = filepath.Join(manifestDir, "cluster-network-02-config.yml")
)

const (

	// We need to manually create our CRD first, so we can create the
	// configuration instance of it.
	// Other operators have their CRD created by the CVO, but we manually
	// create our operator's configuration in the installer.
	netConfigCRD = `
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: networkconfigs.networkoperator.openshift.io
spec:
  group: networkoperator.openshift.io
  names:
    kind: NetworkConfig
    listKind: NetworkConfigList
    plural: networkconfigs
    singular: networkconfig
  scope: Cluster
  versions:
    - name: v1
      served: true
      storage: true
`
)

// Networking generates the cluster-network-*.yml files.
type Networking struct {
	config   *netopv1.NetworkConfig
	FileList []*asset.File
}

var _ asset.WritableAsset = (*Networking)(nil)

// Name returns a human friendly name for the operator.
func (no *Networking) Name() string {
	return "Network Config"
}

// Dependencies returns all of the dependencies directly needed to generate
// network configuration.
func (no *Networking) Dependencies() []asset.Asset {
	return []asset.Asset{
		&installconfig.InstallConfig{},
	}
}

// Generate generates the network operator config and its CRD.
func (no *Networking) Generate(dependencies asset.Parents) error {
	installConfig := &installconfig.InstallConfig{}
	dependencies.Get(installConfig)

	netConfig := installConfig.Config.Networking

	// determine pod address space.
	// This can go away when we get rid of PodCIDR
	// entirely in favor of ClusterNetworks
	var clusterNets []netopv1.ClusterNetwork
	if len(netConfig.ClusterNetworks) > 0 {
		clusterNets = netConfig.ClusterNetworks
	} else if !netConfig.PodCIDR.IPNet.IP.IsUnspecified() {
		clusterNets = []netopv1.ClusterNetwork{
			{
				CIDR:             netConfig.PodCIDR.String(),
				HostSubnetLength: 9,
			},
		}
	} else {
		return errors.Errorf("Either PodCIDR or ClusterNetworks must be specified")
	}

	defaultNet := netopv1.DefaultNetworkDefinition{
		Type: netConfig.Type,
	}

	// Add any network-specific configuration defaults here.
	switch netConfig.Type {
	case netopv1.NetworkTypeOpenshiftSDN:
		defaultNet.OpenshiftSDNConfig = &netopv1.OpenshiftSDNConfig{
			// Default to network policy, operator provides all other defaults.
			Mode: netopv1.SDNModePolicy,
		}
	}

	no.config = &netopv1.NetworkConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: netopv1.SchemeGroupVersion.String(),
			Kind:       "NetworkConfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
			// not namespaced
		},

		Spec: netopv1.NetworkConfigSpec{
			ServiceNetwork:  netConfig.ServiceCIDR.String(),
			ClusterNetworks: clusterNets,
			DefaultNetwork:  defaultNet,
		},
	}

	configData, err := yaml.Marshal(no.config)
	if err != nil {
		return errors.Wrapf(err, "failed to create %s manifests from InstallConfig", no.Name())
	}

	no.FileList = []*asset.File{
		{
			Filename: noCrdFilename,
			Data:     []byte(netConfigCRD),
		},
		{
			Filename: noCfgFilename,
			Data:     configData,
		},
	}

	return nil
}

// Files returns the files generated by the asset.
func (no *Networking) Files() []*asset.File {
	return no.FileList
}

// ClusterNetwork returns the ClusterNetworkingConfig for the ClusterConfig
// object. This is called by ClusterK8sIO, which captures generalized cluster
// state but shouldn't need to be fully networking aware.
func (no *Networking) ClusterNetwork() (*clusterv1a1.ClusterNetworkingConfig, error) {
	if no.config == nil {
		// should be unreachable.
		return nil, errors.Errorf("ClusterNetwork called before initialization")
	}

	pods := []string{}
	for _, cn := range no.config.Spec.ClusterNetworks {
		pods = append(pods, cn.CIDR)
	}

	cn := &clusterv1a1.ClusterNetworkingConfig{
		Services: clusterv1a1.NetworkRanges{
			CIDRBlocks: []string{no.config.Spec.ServiceNetwork},
		},
		Pods: clusterv1a1.NetworkRanges{
			CIDRBlocks: pods,
		},
	}
	return cn, nil
}

// Load loads the already-rendered files back from disk.
func (no *Networking) Load(f asset.FileFetcher) (bool, error) {
	crdFile, err := f.FetchByName(noCrdFilename)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	cfgFile, err := f.FetchByName(noCfgFilename)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, err
	}

	netConfig := &netopv1.NetworkConfig{}
	if err := yaml.Unmarshal(cfgFile.Data, netConfig); err != nil {
		return false, errors.Wrapf(err, "failed to unmarshal %s", noCfgFilename)
	}

	fileList := []*asset.File{crdFile, cfgFile}

	no.FileList, no.config = fileList, netConfig

	return true, nil
}
