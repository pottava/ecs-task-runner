package conf

// AwsConfig is set of AWS configurations
type AwsConfig struct {
	AccountID       string
	Region          *string
	AccessKey       *string
	SecretKey       *string
	Profile         *string
	AssumeRole      *string
	MfaSerialNumber *string
	MfaToken        *string
}

// CommonConfig is set of common configurations
type CommonConfig struct {
	AppVersion     string
	EcsCluster     *string
	ClusterExisted bool
	ExecRoleName   *string
	Timeout        *int64
	ExtendedOutput *bool
	IsDebugMode    bool
}

// RunConfig is set of configurations for running a container
type RunConfig struct {
	Aws            *AwsConfig
	Common         *CommonConfig
	Image          string
	Spot           *bool
	ForceECR       *bool
	TaskDefFamily  *string
	Entrypoint     []*string
	Commands       []*string
	Ports          []*int64
	Environments   map[string]*string
	User           *string
	Labels         map[string]*string
	Subnets        []*string
	SecurityGroups []*string
	CPU            *string
	Memory         *string
	TaskRoleArn    *string
	NumberOfTasks  *int64
	WithParamStore bool
	KMSCustomKeyID *string
	DockerUser     *string
	DockerPassword *string
	AssignPublicIP *bool
	ReadOnlyRootFS *bool
	Asynchronous   *bool
}

// StopConfig is set of configurations for stopping a container
type StopConfig struct {
	Aws       *AwsConfig
	Common    *CommonConfig
	RequestID *string
	TaskARNs  []*string
}
