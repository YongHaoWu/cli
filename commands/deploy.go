package commands

import (
	"io"
	"io/ioutil"
	"strings"

	log "github.com/Sirupsen/logrus"
	enginecontainer "github.com/docker/docker/api/types/container"
	"github.com/projecteru2/cli/types"
	"github.com/projecteru2/cli/utils"
	pb "github.com/projecteru2/core/rpc/gen"
	coreutils "github.com/projecteru2/core/utils"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	cli "gopkg.in/urfave/cli.v2"
	"gopkg.in/yaml.v2"
)

func deploy(c *cli.Context, conn *grpc.ClientConn) {
	pod, entry, image, network, cpu, mem, envs, count := getDeployParams(c)
	if c.NArg() != 1 {
		log.Fatal("[Deploy] no spec")
	}
	specURI := c.Args().First()
	log.Debugf("[Deploy] Deploy %s", specURI)
	var data []byte
	var err error
	if strings.HasPrefix(specURI, "http") {
		data, err = utils.GetSpecFromRemote(specURI)
	} else {
		data, err = ioutil.ReadFile(specURI)
	}
	if err != nil {
		log.Fatalf("[Deploy] read spec failed %v", err)
	}
	client := pb.NewCoreRPCClient(conn)
	opts := generateDeployOpts(data, pod, entry, image, network, cpu, mem, envs, count)
	resp, err := client.CreateContainer(context.Background(), opts)
	if err != nil {
		log.Fatalf("[Deploy] send request failed %v", err)
	}
	for {
		msg, err := resp.Recv()
		if err == io.EOF {
			break
		}

		if err != nil {
			log.Fatalf("[Deploy] Message invalid %v", err)
		}

		if msg.Success {
			log.Infof("[Deploy] Success %s %s %s %v %d", msg.Id, msg.Name, msg.Nodename, msg.Cpu, msg.Memory)
			if len(msg.Hook) > 0 {
				log.Infof("[Deploy] Hook output \n%s", msg.Hook)
			}
			for name, publish := range msg.Publish {
				log.Infof("[Deploy] Bound %s ip %s", name, publish)
			}
		} else {
			log.Errorf("[Deploy] Failed %v", msg.Error)
		}
	}
}

func getDeployParams(c *cli.Context) (string, string, string, string, float64, int64, []string, int32) {
	pod := c.String("pod")
	entry := c.String("entry")
	image := c.String("image")
	network := c.String("network")
	cpu := c.Float64("cpu")
	mem := c.Int64("mem")
	envs := c.StringSlice("env")
	count := int32(c.Int("count"))
	if pod == "" || entry == "" || image == "" {
		log.Fatal("[Deploy] no pod or entry or image")
	}
	return pod, entry, image, network, cpu, mem, envs, count
}

func generateDeployOpts(data []byte, pod, entry, image, network string, cpu float64, mem int64, envs []string, count int32) *pb.DeployOptions {
	specs := &types.Specs{}
	if err := yaml.Unmarshal(data, specs); err != nil {
		log.Fatalf("[generateOpts] get specs failed %v", err)
	}

	networkmode := enginecontainer.NetworkMode(network)
	networks := map[string]string{network: ""}
	if !networkmode.IsUserDefined() {
		networks = map[string]string{}
	}
	entrypoint, ok := specs.Entrypoints[entry]
	if !ok {
		log.Fatal("[generateOpts] get entry failed")
	}

	hook := &pb.HookOptions{}
	if entrypoint.Hook != nil {
		hook.AfterStart = entrypoint.Hook.AfterStart
		hook.BeforeStop = entrypoint.Hook.BeforeStop
		hook.Force = entrypoint.Hook.Force
	}

	healthCheck := &pb.HealthCheckOptions{}
	if entrypoint.HealthCheck != nil {
		healthCheck.Ports = coreutils.DecodePorts(entrypoint.HealthCheck.Ports)
		healthCheck.Url = entrypoint.HealthCheck.URL
		healthCheck.Code = int32(entrypoint.HealthCheck.Code)
	}

	opts := &pb.DeployOptions{
		Name: specs.Appname,
		Entrypoint: &pb.EntrypointOptions{
			Name:          entry,
			Command:       entrypoint.Command,
			Privileged:    entrypoint.Privileged,
			WorkingDir:    entrypoint.WorkingDir,
			LogConfig:     entrypoint.LogConfig,
			Publish:       coreutils.DecodePorts(entrypoint.Publish),
			Healcheck:     healthCheck,
			Hook:          hook,
			RestartPolicy: entrypoint.RestartPolicy,
			ExtraHosts:    entrypoint.ExtraHosts,
		},
		Podname:     pod,
		Image:       image,
		CpuQuota:    cpu,
		Memory:      mem,
		Count:       count,
		Env:         envs,
		Networks:    networks,
		Networkmode: network,
		Volumes:     specs.Volumes,
		Meta:        specs.Meta,
		Dns:         specs.DNS,
	}
	return opts
}

//DeployCommand for running deploy task
func DeployCommand() *cli.Command {
	return &cli.Command{
		Name:  "deploy",
		Usage: "use it to deploy containers by from a image",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "pod",
				Usage: "where to run",
			},
			&cli.StringFlag{
				Name:  "entry",
				Usage: "which entry",
			},
			&cli.StringFlag{
				Name:  "image",
				Usage: "which to run",
			},
			&cli.IntFlag{
				Name:  "count",
				Usage: "how many",
				Value: 1,
			},
			&cli.StringFlag{
				Name:  "network",
				Usage: "SDN name or host mode",
				Value: "host",
			},
			&cli.Float64Flag{
				Name:  "cpu",
				Usage: "how many cpu",
				Value: 1.0,
			},
			&cli.Int64Flag{
				Name:  "mem",
				Usage: "how many memory in bytes",
				Value: 536870912.0,
			},
			&cli.StringSliceFlag{
				Name:  "env",
				Usage: "set env can use multiple times",
			},
		},
		Action: run,
	}
}
