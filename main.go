package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"syscall"

	flag "github.com/spf13/pflag"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/terminal"

	remouse "github.com/kevinconway/remouse/pkg"
)

func main() {

	driver := &remouse.RobotgoDriver{}

	fs := flag.NewFlagSet("remouse", flag.ExitOnError)
	orientation := fs.String("orientation", "right", "Orientation of the tablet. Choices are vertical, right, and left")
	tabletHeight := fs.Int("tablet-height", remouse.DefaultTabletHeight, "The max units per millimeter for the hight of the tablet. Probably don't change this.")
	tabletWidth := fs.Int("tablet-width", remouse.DefaultTabletWidth, "The max units per millimeter for the width of the tablet. Probably don't change this.")
	tmpScreenWidth, tmpScreenHeight, _ := driver.GetSize()
	screenHeight := fs.Int("screen-height", tmpScreenHeight, "The max units per millimeter of the host screen height. Probably don't change this.")
	screenWidth := fs.Int("screen-width", tmpScreenWidth, "The max units per millimeter of the host screen width. Probably don't change this.")
	sshIP := fs.String("ssh-ip", "10.11.99.1:22", "The host and port of a tablet.")
	sshUser := fs.String("ssh-user", "root", "The ssh username to use when logging into the tablet.")
	sshPassword := fs.String("ssh-password", "", "An optional password to use when ssh-ing into the tablet. Use - for a prompt rather than entering a value. If not given then public/private keypair authentication is used.")
	sshSocket := fs.String("ssh-socket", os.Getenv("SSH_AUTH_SOCK"), "Path to the SSH auth socket. This must not be empty if using public/private keypair authentication.")
	evtFile := fs.String("event-file", "/dev/input/event0", "The path on the tablet from which to read evdev events. Probably don't change this.")
	_ = fs.Parse(os.Args[1:])

	if *sshPassword == "-" {
		fmt.Print("Enter Password: ")
		pwd, err := terminal.ReadPassword(int(syscall.Stdin))
		if err != nil {
			panic(err)
		}
		*sshPassword = string(pwd)
	}
	sshConfig := &ssh.ClientConfig{
		User: *sshUser,
		Auth: []ssh.AuthMethod{
			ssh.Password(*sshPassword),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	if *sshPassword == "" {
		agentFd, err := net.Dial("unix", *sshSocket)
		if err != nil {
			panic(err)
		}
		defer agentFd.Close()

		agentSigner := agent.NewClient(agentFd)

		sshConfig = &ssh.ClientConfig{
			User: *sshUser,
			Auth: []ssh.AuthMethod{
				ssh.PublicKeysCallback(agentSigner.Signers),
			},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}
	}

	client, err := ssh.Dial("tcp", *sshIP, sshConfig)
	if err != nil {
		panic(err)
	}

	sesh, err := client.NewSession()
	if err != nil {
		panic(err)
	}
	defer sesh.Close()

	pipe, err := sesh.StdoutPipe()
	if err != nil {
		panic(err)
	}
	if err = sesh.Start(fmt.Sprintf("cat %s", *evtFile)); err != nil {
		panic(err)
	}

	it := &remouse.SelectingEvdevIterator{
		Wrapped: &remouse.FileEvdevIterator{
			Source: ioutil.NopCloser(pipe),
		},
		Selection: []uint16{remouse.EV_ABS},
	}
	defer it.Close()

	sm := &remouse.EvdevStateMachine{
		Iterator:          it,
		PressureThreshold: 1000,
	}
	defer sm.Close()

	var sc remouse.PositionScaler
	switch *orientation {
	case "right":
		sc = &remouse.RightPositionScaler{
			TabletWidth:  *tabletWidth,
			TabletHeight: *tabletHeight,
			ScreenWidth:  *screenWidth,
			ScreenHeight: *screenHeight,
		}
	case "left":
		sc = &remouse.LeftPositionScaler{
			TabletWidth:  *tabletWidth,
			TabletHeight: *tabletHeight,
			ScreenWidth:  *screenWidth,
			ScreenHeight: *screenHeight,
		}
	case "vertical":
		sc = &remouse.VerticalPositionScaler{
			TabletWidth:  *tabletWidth,
			TabletHeight: *tabletHeight,
			ScreenWidth:  *screenWidth,
			ScreenHeight: *screenHeight,
		}
	default:
		panic(fmt.Sprintf("unknown orienation selection %s", *orientation))
	}

	rt := &remouse.Runtime{
		PositionScaler: sc,
		StateMachine:   sm,
		Driver:         driver,
	}

	fmt.Println("Remouse connected and running.")
	for rt.Next() {
	}
	if err = rt.Close(); err != nil {
		panic(err)
	}
}
