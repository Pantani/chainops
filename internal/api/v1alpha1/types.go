package v1alpha1

const (
	APIVersion       = "bgorch.io/v1alpha1"
	KindChainCluster = "ChainCluster"
)

type ChainCluster struct {
	APIVersion string           `yaml:"apiVersion" json:"apiVersion"`
	Kind       string           `yaml:"kind" json:"kind"`
	Metadata   ObjectMeta       `yaml:"metadata" json:"metadata"`
	Spec       ChainClusterSpec `yaml:"spec" json:"spec"`
}

type ObjectMeta struct {
	Name   string            `yaml:"name" json:"name"`
	Labels map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
}

type ChainClusterSpec struct {
	Family       string         `yaml:"family" json:"family"`
	Profile      string         `yaml:"profile,omitempty" json:"profile,omitempty"`
	Plugin       string         `yaml:"plugin" json:"plugin"`
	Runtime      RuntimeSpec    `yaml:"runtime" json:"runtime"`
	NodePools    []NodePoolSpec `yaml:"nodePools" json:"nodePools"`
	PluginConfig PluginConfig   `yaml:"pluginConfig,omitempty" json:"pluginConfig,omitempty"`
	Backup       BackupPolicy   `yaml:"backup,omitempty" json:"backup,omitempty"`
	Upgrade      UpgradePolicy  `yaml:"upgrade,omitempty" json:"upgrade,omitempty"`
	Observe      ObservePolicy  `yaml:"observe,omitempty" json:"observe,omitempty"`
}

type RuntimeSpec struct {
	Backend       string        `yaml:"backend" json:"backend"`
	Target        string        `yaml:"target,omitempty" json:"target,omitempty"`
	BackendConfig BackendConfig `yaml:"backendConfig,omitempty" json:"backendConfig,omitempty"`
}

type BackendConfig struct {
	Compose    *ComposeConfig    `yaml:"compose,omitempty" json:"compose,omitempty"`
	SSHSystemd *SSHSystemdConfig `yaml:"sshSystemd,omitempty" json:"sshSystemd,omitempty"`
}

type ComposeConfig struct {
	ProjectName string `yaml:"projectName,omitempty" json:"projectName,omitempty"`
	NetworkName string `yaml:"networkName,omitempty" json:"networkName,omitempty"`
	OutputFile  string `yaml:"outputFile,omitempty" json:"outputFile,omitempty"`
}

type SSHSystemdConfig struct {
	User string `yaml:"user,omitempty" json:"user,omitempty"`
	Port int    `yaml:"port,omitempty" json:"port,omitempty"`
}

type NodePoolSpec struct {
	Name     string   `yaml:"name" json:"name"`
	Replicas int      `yaml:"replicas,omitempty" json:"replicas,omitempty"`
	Roles    []string `yaml:"roles,omitempty" json:"roles,omitempty"`
	Template NodeSpec `yaml:"template" json:"template"`
}

type NodeSpec struct {
	Name         string         `yaml:"name,omitempty" json:"name,omitempty"`
	Role         string         `yaml:"role,omitempty" json:"role,omitempty"`
	Workloads    []WorkloadSpec `yaml:"workloads" json:"workloads"`
	Volumes      []VolumeSpec   `yaml:"volumes,omitempty" json:"volumes,omitempty"`
	Files        []FileSpec     `yaml:"files,omitempty" json:"files,omitempty"`
	Secrets      []SecretRef    `yaml:"secrets,omitempty" json:"secrets,omitempty"`
	Sync         SyncPolicy     `yaml:"sync,omitempty" json:"sync,omitempty"`
	Resources    ResourceSizing `yaml:"resources,omitempty" json:"resources,omitempty"`
	Hooks        LifecycleHooks `yaml:"hooks,omitempty" json:"hooks,omitempty"`
	PluginConfig PluginConfig   `yaml:"pluginConfig,omitempty" json:"pluginConfig,omitempty"`
}

type WorkloadMode string

const (
	WorkloadModeContainer WorkloadMode = "container"
	WorkloadModeHost      WorkloadMode = "host"
)

type WorkloadSpec struct {
	Name          string            `yaml:"name" json:"name"`
	Mode          WorkloadMode      `yaml:"mode,omitempty" json:"mode,omitempty"`
	Image         string            `yaml:"image,omitempty" json:"image,omitempty"`
	Binary        string            `yaml:"binary,omitempty" json:"binary,omitempty"`
	Command       []string          `yaml:"command,omitempty" json:"command,omitempty"`
	Args          []string          `yaml:"args,omitempty" json:"args,omitempty"`
	Env           []EnvVar          `yaml:"env,omitempty" json:"env,omitempty"`
	Ports         []PortSpec        `yaml:"ports,omitempty" json:"ports,omitempty"`
	VolumeMounts  []VolumeMountSpec `yaml:"volumeMounts,omitempty" json:"volumeMounts,omitempty"`
	HealthChecks  []HealthCheckSpec `yaml:"healthChecks,omitempty" json:"healthChecks,omitempty"`
	RestartPolicy RestartPolicy     `yaml:"restartPolicy,omitempty" json:"restartPolicy,omitempty"`
	Resources     ResourceSizing    `yaml:"resources,omitempty" json:"resources,omitempty"`
	DependsOn     []string          `yaml:"dependsOn,omitempty" json:"dependsOn,omitempty"`
	PluginConfig  PluginConfig      `yaml:"pluginConfig,omitempty" json:"pluginConfig,omitempty"`
}

type EnvVar struct {
	Name  string `yaml:"name" json:"name"`
	Value string `yaml:"value,omitempty" json:"value,omitempty"`
}

type PortSpec struct {
	Name          string `yaml:"name,omitempty" json:"name,omitempty"`
	ContainerPort int    `yaml:"containerPort" json:"containerPort"`
	HostPort      int    `yaml:"hostPort,omitempty" json:"hostPort,omitempty"`
	Protocol      string `yaml:"protocol,omitempty" json:"protocol,omitempty"`
}

type VolumeType string

const (
	VolumeTypeNamed VolumeType = "named"
	VolumeTypeBind  VolumeType = "bind"
)

type VolumeSpec struct {
	Name   string     `yaml:"name" json:"name"`
	Type   VolumeType `yaml:"type,omitempty" json:"type,omitempty"`
	Source string     `yaml:"source,omitempty" json:"source,omitempty"`
}

type VolumeMountSpec struct {
	Volume   string `yaml:"volume" json:"volume"`
	Path     string `yaml:"path" json:"path"`
	ReadOnly bool   `yaml:"readOnly,omitempty" json:"readOnly,omitempty"`
}

type FileSpec struct {
	Path    string `yaml:"path" json:"path"`
	Content string `yaml:"content" json:"content"`
	Mode    string `yaml:"mode,omitempty" json:"mode,omitempty"`
}

type SecretRef struct {
	Name       string `yaml:"name" json:"name"`
	Source     string `yaml:"source" json:"source"`
	Key        string `yaml:"key,omitempty" json:"key,omitempty"`
	TargetEnv  string `yaml:"targetEnv,omitempty" json:"targetEnv,omitempty"`
	TargetFile string `yaml:"targetFile,omitempty" json:"targetFile,omitempty"`
}

type SyncPolicy struct {
	Mode     string `yaml:"mode,omitempty" json:"mode,omitempty"`
	Snapshot string `yaml:"snapshot,omitempty" json:"snapshot,omitempty"`
}

type BackupPolicy struct {
	Enabled   bool   `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Schedule  string `yaml:"schedule,omitempty" json:"schedule,omitempty"`
	Retention int    `yaml:"retention,omitempty" json:"retention,omitempty"`
}

type UpgradePolicy struct {
	Strategy       string `yaml:"strategy,omitempty" json:"strategy,omitempty"`
	MaxUnavailable int    `yaml:"maxUnavailable,omitempty" json:"maxUnavailable,omitempty"`
}

type ObservePolicy struct {
	Metrics bool `yaml:"metrics,omitempty" json:"metrics,omitempty"`
	Logs    bool `yaml:"logs,omitempty" json:"logs,omitempty"`
}

type ResourceSizing struct {
	CPU    string `yaml:"cpu,omitempty" json:"cpu,omitempty"`
	Memory string `yaml:"memory,omitempty" json:"memory,omitempty"`
}

type LifecycleHooks struct {
	PreStart  []string `yaml:"preStart,omitempty" json:"preStart,omitempty"`
	PostStart []string `yaml:"postStart,omitempty" json:"postStart,omitempty"`
	PreStop   []string `yaml:"preStop,omitempty" json:"preStop,omitempty"`
}

type HealthCheckType string

const (
	HealthCheckCMD  HealthCheckType = "cmd"
	HealthCheckHTTP HealthCheckType = "http"
	HealthCheckTCP  HealthCheckType = "tcp"
)

type HealthCheckSpec struct {
	Type             HealthCheckType `yaml:"type" json:"type"`
	Command          string          `yaml:"command,omitempty" json:"command,omitempty"`
	Path             string          `yaml:"path,omitempty" json:"path,omitempty"`
	Port             int             `yaml:"port,omitempty" json:"port,omitempty"`
	InitialDelaySec  int             `yaml:"initialDelaySec,omitempty" json:"initialDelaySec,omitempty"`
	PeriodSec        int             `yaml:"periodSec,omitempty" json:"periodSec,omitempty"`
	FailureThreshold int             `yaml:"failureThreshold,omitempty" json:"failureThreshold,omitempty"`
}

type RestartPolicy string

const (
	RestartAlways        RestartPolicy = "always"
	RestartUnlessStopped RestartPolicy = "unless-stopped"
	RestartOnFailure     RestartPolicy = "on-failure"
)

type PluginConfig struct {
	GenericProcess *GenericProcessConfig `yaml:"genericProcess,omitempty" json:"genericProcess,omitempty"`
}

type GenericProcessConfig struct {
	ExtraFiles []FileSpec `yaml:"extraFiles,omitempty" json:"extraFiles,omitempty"`
}
