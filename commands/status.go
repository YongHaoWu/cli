package commands

import (
	"encoding/json"
	"io"

	pb "github.com/projecteru2/core/rpc/gen"
	"github.com/projecteru2/core/store"
	coretypes "github.com/projecteru2/core/types"
	coreutils "github.com/projecteru2/core/utils"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	cli "gopkg.in/urfave/cli.v2"
)

func status(c *cli.Context) error {
	client := setupAndGetGRPCConnection().GetRPCClient()
	name := c.Args().First()
	entry := c.String("entry")
	node := c.String("node")
	labels := makeLabels(c.StringSlice("label"))

	resp, err := client.DeployStatus(
		context.Background(),
		&pb.DeployStatusOptions{
			Appname:    name,
			Entrypoint: entry,
			Nodename:   node,
		})
	if err != nil || resp == nil {
		cli.Exit("", -1)
	}

	for {
		msg, err := resp.Recv()
		if err == io.EOF {
			break
		}

		if err != nil || msg == nil {
			cli.Exit("", -1)
		}

		container, err := client.GetContainer(context.TODO(), &pb.ContainerID{Id: msg.Id})
		if err != nil {
			log.Errorf("[status] get container %s failed %v", msg.Id, err)
			continue
		}

		if !coreutils.FilterContainer(container.Labels, labels) {
			log.Debugf("[status] ignore container %s", container.Id)
			continue
		}

		containerStatus := &coretypes.ContainerStatus{}
		if len(msg.Data) > 0 {
			if err := json.Unmarshal(msg.Data, containerStatus); err != nil {
				log.Errorf("[status] parse container data failed %v", err)
				break
			}
		}

		if msg.Action == store.DeleteEvent {
			log.Infof("[%s] %s deleted", coreutils.ShortID(container.Id), container.Name)
			continue
		}

		if containerStatus.Healthy {
			log.Infof("[%s] %s on %s back to life", coreutils.ShortID(container.Id), container.Name, msg.Nodename)
			for networkName, addrs := range container.Publish {
				log.Infof("[%s] published at %s bind %v", coreutils.ShortID(container.Id), networkName, addrs)
			}
			continue
		}
		log.Warnf("[%s] %s on %s is unhealthy", coreutils.ShortID(container.Id), container.Name, msg.Nodename)
	}
	return nil
}

// StatusCommand show status
func StatusCommand() *cli.Command {
	return &cli.Command{
		Name:      "status",
		Usage:     "get deploy status from core",
		ArgsUsage: "name can be none",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "entry",
				Usage: "entry filter or not",
			},
			&cli.StringFlag{
				Name:  "node",
				Usage: "node filter or not",
			},
			&cli.StringSliceFlag{
				Name:  "label",
				Usage: "label filter can set multiple times",
			},
		},
		Action: status,
	}
}
