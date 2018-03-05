package main

import (
	"os"
	//"strconv"
	"bytes"
	"os/exec"
	//"syscall"
	"flag"
	"fmt"
	"github.com/golang/glog"
	"io/ioutil"
	"net"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1alpha"
)

//const (
var onloadver string    = "201606-u1.3"
var onloadsrc string    = "http://www.openonload.org/download/openonload-" + onloadver + ".tgz"
var regExpCAM string    = "(?m)[\r\n]+^.*Synopsys.*$"
var socketName string   = "camCard"
var resourceName string = "cambricon/card"
var k8sAPI string       = "https://localhost:6443"
var nodeLabelVersion string = "device.sfc.onload-version"

//)
// camCardManager manages Cambricon Card devices
type camCardManager struct {
	devices     map[string]*pluginapi.Device
	deviceFiles []string
}

func NewCAMCardManager() (*camCardManager, error) {
	return &camCardManager{
		devices:     make(map[string]*pluginapi.Device),
		deviceFiles: []string{"/dev/cambricon_dmDev0"},
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

func (cam *camCardManager) discoverCambriconResources() bool {
	found := false
	cam.devices = make(map[string]*pluginapi.Device)
	glog.Info("discoverCambriconResources")
	out, err := ExecCommand("lshw", "-short", "-class", "generic")
	if err != nil {
		glog.Errorf("Error while discovering: %v", err)
		return found
	}
	re := regexp.MustCompile(regExpCAM)
	camCards := re.FindAllString(out.String(), -1)
	for _, nic := range camCards {
		//nameIf := strings.Fields(nic)[1]
		fmt.Printf("logical name: %v \n", strings.Fields(nic)[2])
		dev := pluginapi.Device{ID: strings.Fields(nic)[2], Health: pluginapi.Healthy}
		cam.devices[strings.Fields(nic)[2]] = &dev
		found = true
	}
	fmt.Printf("Devices: %v \n", cam.devices)

	return found
}

func (cam *camCardManager) isOnloadInstallHealthy() bool {
	healthy := false
	//cmdName := "onload"
	//cmdName := "ssh"
	//out, _ := ExecCommand(cmdName, "--version")
	//out, _ := ExecCommand(cmdName, "-o", "StrictHostKeyChecking=no", "127.0.0.1", "onload --version")

	//if strings.Contains(out.String(), "Cambricon Communications") && strings.Contains(out.String(), onloadver) {
		//cmdName = "/sbin/ldconfig"
	//	out, _ := ExecCommand(cmdName, "-o", "StrictHostKeyChecking=no", "127.0.0.1", "/sbin/ldconfig -N -v")

	//	if strings.Contains(out.String(), "libonload") {
	//		if AreAllOnloadDevicesAvailable() == true {
	//			fmt.Println("All Onload devices Verified\n")
				healthy = true
	//		} else {
	//			fmt.Errorf("Inconsistent Onload installation. All Onload devices are not available!!!")
	//		}
	//	} else {
	//		fmt.Errorf("Inconsistent Onload installation. libonload not detected.")
	//	}
	//} else {
	//	fmt.Errorf("Inconsistent Onload installation.")
	//}
	return healthy
}

//func (cam *camCardManager) installOnload() error {
//
//
//
//}

//func (cam *camCardManager) Init() error {
//	glog.Info("Init\n")
//	err := cam.installOnload()
//	return err
//}

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
		if !cam.isOnloadInstallHealthy() {
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
	//	containerName := strings.Join([]string{"k8s", "POD", rqt.PodName, rqt.Namespace}, "_")
	for _, id := range rqt.DevicesIDs {
		if _, ok := cam.devices[id]; ok {
			for _, d := range cam.deviceFiles {
				resp.Devices = append(resp.Devices, &pluginapi.DeviceSpec{
					HostPath:      d,
					ContainerPath: d,
					Permissions:   "mrw",
				})
			}
			glog.Info("Allocated interface ", id)
			//glog.Info("Allocate interface ", id, " to ", containerName)
			//go MoveInterface(id)
		}
	}
	return resp, nil
}

func MoveInterface(interfaceName string) {
	glog.Info("move interface after reading checkpoint file")
	_, err := ExecCommand("/usr/bin/cont-cam-nic-move.sh", interfaceName, k8sAPI)
	if err != nil {
		glog.Error(err)
	}
}

func AnnotateNodeWithOnloadVersion(version string) {
	glog.Info("Annotating Node with onload version: ", version, " ", nodeLabelVersion)
	//TODO: Read api url from config map
	out, err := ExecCommand("/usr/bin/annotate_node.sh", k8sAPI, nodeLabelVersion, version)
	if err != nil {
		glog.Error(err)
	}
	glog.Info(out.String())
}

func AreAllOnloadDevicesAvailable() bool {
	glog.Info("AreAllOnloadDevicesAvailable\n")

	found := 0

	// read the whole file at once
	b, err := ioutil.ReadFile("/gopath/proc/devices")
	if err != nil {
		panic(err)
	}
	s := string(b)

	if strings.Index(s, "onload_epoll") > 0 {
		found++
	}

	if strings.Index(s, "onload_cplane") > 0 {
		found++
	}

	// '\n' is added to avoid a match with onload_cplane and onload_epoll
	if strings.Index(s, "onload\n") > 0 {
		found++
	}

	if found == 3 {
		return true
	} else {
		return false
	}
}

func (cam *camCardManager) UnInit() {
	var out bytes.Buffer
	var stderr bytes.Buffer

	//fmt.Println("CMD--" + cmdName + ": " + out.String())
	cmdName := "onload_uninstall"
	cmd := exec.Command(cmdName)
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		fmt.Println("CMD--" + cmdName + ": " + fmt.Sprint(err) + ": " + stderr.String())
	}
	//fmt.Println("CMD--" + cmdName + ": " + out.String())

	return
}

func main() {
	flag.Parse()
	fmt.Printf("Starting main \n")

	//onloadver = os.Args[1] //"201606-u1.3"
	//onloadsrc = os.Args[2]    //"http://www.openonload.org/download/openonload-" + onloadver + ".tgz"
	//regExpCAM = os.Args[2]    //"(?m)[\r\n]+^.*CAM[6-9].*$"
	//socketName = os.Args[3]   //"camCard"
	//resourceName = os.Args[4] //"pod.alpha.kubernetes.io/opaque-int-resource-camCard"
	//k8sAPI = os.Args[5]
	//nodeLabelVersion = os.Args[6]
	flag.Lookup("logtostderr").Value.Set("true")

	cam, err := NewCAMCardManager()
	if err != nil {
		glog.Fatal(err)
		os.Exit(1)
	}

	found := cam.discoverCambriconResources()
	if !found {
		// clean up any exisiting device plugin software
		//cam.UnInit()
		glog.Errorf("No Cambricon Cards are present\n")
		os.Exit(1)
	}
	if !cam.isOnloadInstallHealthy() {
		//err = cam.Init()
		//if err != nil {
		glog.Errorf("Error with onload installation")
		//		for _, device := range cam.devices {
		//			device.Health = pluginapi.Unhealthy
		//		}
		//	}
		//AnnotateNodeWithOnloadVersion("")
	}
	//AnnotateNodeWithOnloadVersion(onloadver)

	pluginEndpoint := fmt.Sprintf("%s-%d.sock", socketName, time.Now().Unix())
	//serverStarted := make(chan bool)
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
