package main

import (
	"os"
	"bytes"
	"os/exec"
	"flag"
	"fmt"
	"github.com/golang/glog"
	"io/ioutil"
	"net"
	"path"
	"strings"
	"sync"
	"time"
	"github.com/satori/go.uuid"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1alpha"
)

//const (
var socketName string   = "camCard"
var resourceName string = "cambricon/card"
var volumePath string   = "/home/a/fyk/container-volume"
//)
// camCardManager manages Cambricon Card devices
type camCardManager struct {
	devices     map[string]*pluginapi.Device
	deviceFiles map[string]string
}

func NewCAMCardManager() (*camCardManager, error) {
	return &camCardManager{
		devices:     make(map[string]*pluginapi.Device),
		deviceFiles: make(map[string]string),
	}, nil
}

func ExecCommand(cmdName string, arg ...string) (bytes.Buffer, error) {
	var out bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.Command(cmdName, arg...)
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		fmt.Println("CMD--" + cmdName + ": " + fmt.Sprint(err) + ": " + stderr.String())
	}

	return out, err
}

func ListDir(dirPth string, suffix string) (files []string, err error) {
	files = make([]string, 0, 10)
	dir, err := ioutil.ReadDir(dirPth)
	if err != nil {
		return nil, err
	}
	PthSep := string(os.PathSeparator)
	for _, fi := range dir {
		if fi.IsDir() { // ignore dirs
			continue
		}
		if strings.Contains(fi.Name(), suffix) { //match files
			files = append(files, dirPth+PthSep+fi.Name())
		}
	}

	return files, nil
}

func (cam *camCardManager) doesExist(devpath string) bool {

	result := false
	for _, v := range cam.deviceFiles{
		//fmt.Printf("exist card is : %s\n", v)
		//fmt.Printf("check dev is : %s\n", devpath)
		if 0 == strings.Compare(v, devpath){
			result = true
			break
		}
	}

	return  result
}

func (cam *camCardManager) discoverCambriconResources() bool {
	found := false
	glog.Info("discover Cambricon Card Resources")

	camCards, err := ListDir("/dev", "cambricon")

	if err != nil {
		glog.Errorf("Error while discovering: %v", err)
		return found
	}

	for _, card := range camCards {
		//fmt.Printf("detect card is: %s \n", card)
		//fmt.Printf("check result is %s\n", cam.doesExist(card))
		if cam.doesExist(card) == false {
			//fmt.Printf("devicefiles %s", cam.deviceFiles)
			u1 := uuid.Must(uuid.NewV4())
			fmt.Printf("Creating UUID for %s : %s\n", card, u1)
			out := fmt.Sprint(u1)
			dev := pluginapi.Device{ID: out, Health: pluginapi.Healthy}
			cam.devices[out] = &dev
			cam.deviceFiles[out] = card
			fmt.Printf("devicefiles %s\n", cam.deviceFiles)
		}
	}

	fmt.Printf("Devices: %v \n", cam.devices)
	if len(cam.deviceFiles) > 0{
		found = true
	}

	return found
}

func (cam *camCardManager) isHealthy() bool {
	healthy := false

	healthy = true

	return healthy
}

func Register(kubeletEndpoint string, pluginEndpoint, socketName string) error {
	conn, err := grpc.Dial(kubeletEndpoint, grpc.WithInsecure(),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}))
	defer conn.Close()
	if err != nil {
		return fmt.Errorf("device-plugin: cannot connect to kubelet service: %v", err)
	}
	client := pluginapi.NewRegistrationClient(conn)
	reqt := &pluginapi.RegisterRequest{
		Version:      pluginapi.Version,
		Endpoint:     pluginEndpoint,
		ResourceName: resourceName,
	}

	_, err = client.Register(context.Background(), reqt)
	if err != nil {
		return fmt.Errorf("device-plugin: cannot register to kubelet service: %v", err)
	}
	return nil
}

// Implements DevicePlugin service functions
func (cam *camCardManager) ListAndWatch(emtpy *pluginapi.Empty, stream pluginapi.DevicePlugin_ListAndWatchServer) error {
	glog.Info("device-plugin: ListAndWatch start\n")
	for {
		cam.discoverCambriconResources()
		if !cam.isHealthy() {
			glog.Errorf("Error with onload installation. Marking devices unhealthy.")
			for _, device := range cam.devices {
				device.Health = pluginapi.Unhealthy
			}
		}
		resp := new(pluginapi.ListAndWatchResponse)
		for _, dev := range cam.devices {
			glog.Info("dev ", dev)
			resp.Devices = append(resp.Devices, dev)
		}
		glog.Info("resp.Devices ", resp.Devices)
		if err := stream.Send(resp); err != nil {
			glog.Errorf("Failed to send response to kubelet: %v\n", err)
		}
		time.Sleep(5 * time.Second)
	}
	return nil
}

func (cam *camCardManager) Allocate(ctx context.Context, rqt *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	glog.Info("Allocate")
	resp := new(pluginapi.AllocateResponse)
	// mount devices to container
	for _, id := range rqt.DevicesIDs {
		if _, ok := cam.devices[id]; ok {
			if d, ok :=cam.deviceFiles[id]; ok{
				resp.Devices = append(resp.Devices, &pluginapi.DeviceSpec{
					HostPath:      d,
					ContainerPath: d,
					Permissions:   "mrw",
				})
			}
		}
	}
	// mount volume to container
	resp.Mounts = append(resp.Mounts, &pluginapi.Mount{
		HostPath: volumePath,
		ContainerPath: volumePath,
	})
	return resp, nil
}

func (cam *camCardManager) Init() error {
        glog.Info("Init\n")
        glog.Info("Install kernel module\n")

        // install kernel module
        out, err := ExecCommand("./load.sh")
        if err != nil {
               glog.Error(out)
        }

        return err
}


func main() {
	flag.Parse()
	fmt.Printf("Starting Cambricon device plugin main \n")

	flag.Lookup("logtostderr").Value.Set("true")

	cam, err := NewCAMCardManager()
	if err != nil {
		glog.Fatal(err)
		os.Exit(1)
	}

	found := cam.discoverCambriconResources()
	if !found {
		glog.Errorf("No Cambricon Cards are present\n")
		os.Exit(1)
	}

	if !cam.isHealthy() {
		glog.Errorf("Error with onload installation")
	}

	pluginEndpoint := fmt.Sprintf("%s-%d.sock", socketName, time.Now().Unix())

	var wg sync.WaitGroup
	wg.Add(1)
	// Starts device plugin service.
	go func() {
		defer wg.Done()
		fmt.Printf("DveicePluginPath %s, pluginEndpoint %s\n", pluginapi.DevicePluginPath, pluginEndpoint)
		fmt.Printf("device-plugin start server at: %s\n", path.Join(pluginapi.DevicePluginPath, pluginEndpoint))
		lis, err := net.Listen("unix", path.Join(pluginapi.DevicePluginPath, pluginEndpoint))
		if err != nil {
			glog.Fatal(err)
			return
		}
		grpcServer := grpc.NewServer()
		pluginapi.RegisterDevicePluginServer(grpcServer, cam)
		grpcServer.Serve(lis)
	}()

	// TODO: fix this
	time.Sleep(5 * time.Second)
	// Registers with Kubelet.
	err = Register(pluginapi.KubeletSocket, pluginEndpoint, resourceName)
	if err != nil {
		glog.Fatal(err)
	}
	fmt.Printf("device-plugin registered\n")
	wg.Wait()
}
