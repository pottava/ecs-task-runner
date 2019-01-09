package etr

// AwsConfig is set of AWS configurations
type AwsConfig struct {
	accountID string
	AccessKey *string
	SecretKey *string
	Region    *string
}

// CommonConfig is set of common configurations
type CommonConfig struct {
	EcsCluster     *string
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
	ForceECR       *bool
	TaskDefFamily  *string
	Entrypoint     []*string
	Commands       []*string
	Ports          []*int64
	Environments   map[string]*string
	Labels         map[string]*string
	Subnets        []*string
	SecurityGroups []*string
	CPU            *string
	Memory         *string
	TaskRoleArn    *string
	NumberOfTasks  *int64
	withParamStore *bool
	KMSCustomKeyID *string
	DockerUser     *string
	DockerPassword *string
	AssignPublicIP *bool
	Asynchronous   *bool
}

// StopConfig is set of configurations for stopping a container
type StopConfig struct {
	Aws       *AwsConfig
	Common    *CommonConfig
	RequestID *string
	TaskARNs  []*string
}
