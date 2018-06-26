package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"9fans.net/go/plan9"
	"9fans.net/go/plan9/client"
)

func TestMain(m *testing.M) {
	switch os.Getenv("TEST_MAIN") {
	case "edwood":
		main()
	default:
		// TODO: Replace Xvfb with a fake devdraw.
		var x *exec.Cmd
		switch runtime.GOOS {
		case "linux", "freebsd", "openbsd", "netbsd", "dragonfly":
			if os.Getenv("DISPLAY") == "" {
				dp := fmt.Sprintf(":%d", xvfbServerNumber())
				x = exec.Command("Xvfb", dp)
				if err := x.Start(); err != nil {
					log.Fatalf("failed to execute Xvfb: %v", err)
				}
				// Give Xvfb some time to start up.
				// 3 seconds is default for xvfb-run.
				time.Sleep(3 * time.Second)
				os.Setenv("DISPLAY", dp)
			}
		}
		e := m.Run()

		if x != nil {
			// Kill Xvfb gracefully, so that it cleans up the /tmp/.X*-lock file.
			x.Process.Signal(os.Interrupt)
			x.Wait()
		}
		os.Exit(e)
	}
}

// XvfbServerNumber finds a free server number for Xfvb.
// Similar logic is used by /usr/bin/xvfb-run:/^find_free_servernum/
func xvfbServerNumber() int {
	for n := 99; n < 1000; n++ {
		if _, err := os.Stat(fmt.Sprintf("/tmp/.X%d-lock", n)); os.IsNotExist(err) {
			return n
		}
	}
	panic("no free X server number")
}

type Acme struct {
	t    *testing.T
	ns   string
	cmd  *exec.Cmd
	fsys *client.Fsys
}

// augmentPath extends PATH so that plan9 dependencies can be
// found in the build directory.
func augmentPathEnv() {
	// We only have Linux executables.
	if runtime.GOOS != "linux" {
		return
	}

	// If the executables are already present, skip.
	_, errdevdraw := exec.LookPath("devdraw")
	_, err9pserve := exec.LookPath("9pserve")
	if errdevdraw == nil && err9pserve == nil {
		return
	}

	wd, err := os.Getwd()
	if err != nil {
		return
	}

	path := os.Getenv("PATH") + ":" + filepath.Join(wd, "build", "bin")
	os.Setenv("PATH", path)

	// We also need fonts.
	if _, hzp9 := os.LookupEnv("PLAN9"); !hzp9 {
		os.Setenv("PLAN9", filepath.Join(wd, "build"))
	}
}

// startAcme runs an edwood process and 9p mounts it (at acme) in the
// namespace so that a test may exercise IPC to the subordinate edwood
// process.
func startAcme(t *testing.T) *Acme {
	// If $USER is not set (i.e. running in a Docker container)
	// MountService will fail. Detect this and give up if this is so.
	if _, hzuser := os.LookupEnv("USER"); !hzuser {
		t.Fatalf("Test will fail unless USER is set in environment. Please set.")
	}

	ns, err := ioutil.TempDir("", "ns.fsystest")
	if err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}
	os.Setenv("NAMESPACE", ns)
	augmentPathEnv()

	acmd := exec.Command(os.Args[0])
	acmd.Env = append(os.Environ(), "TEST_MAIN=edwood")

	acmd.Stdout = os.Stdout
	acmd.Stderr = os.Stderr
	if err := acmd.Start(); err != nil {
		t.Fatalf("failed to execute edwood: %v", err)
	}

	var fsys *client.Fsys
	for i := 0; i < 10; i++ {
		fsys, err = client.MountService("acme")
		if err != nil {
			if i >= 9 {
				t.Fatalf("Failed to mount acme: %v", err)
				return nil
			} else {
				time.Sleep(time.Second)
			}
		} else {
			break
		}
	}
	return &Acme{
		ns:   ns,
		cmd:  acmd,
		fsys: fsys,
	}
}

func (a *Acme) Cleanup() {
	a.cmd.Process.Kill()
	a.cmd.Wait()
	if err := os.RemoveAll(a.ns); err != nil {
		a.t.Errorf("failed to remove temporary namespace %v: %v", a.ns, err)
	}
}

// Fsys tests run my running a server and client in-process and communicating
// externally.

func TestFSys(t *testing.T) {
	var err error

	a := startAcme(t)
	defer a.Cleanup()
	fsys := a.fsys

	/*	fid, err := fsys.Open("/", 0) // Readonly
		if err != nil {
			t.Errorf("Failed to open/: %v", err)
		}

		dirs, err := fid.Dirread()
		if err != nil {
			t.Errorf("Failed to open/: %v", err)
		}
		for _, d := range dirs {
			fmt.Printf("%v\n", d.String())
		}
		fid.Close()

		fid, err = fsys.Open("/1", plan9.OREAD)
		if err != nil {
			t.Errorf("Failed to walk to /1: %v", err)
		}
		dirs, err = fid.Dirread()
		if err != nil {
			t.Errorf("Failed to open/: %v", err)
		}
		for _, d := range dirs {
			fmt.Printf("%v\n", d.String())
		}
	*/
	fid, err := fsys.Open("/new/body", plan9.OWRITE)
	if err != nil {
		t.Errorf("Failed to open/: %v", err)
	}
	text := []byte("This is a test\nof the emergency typing system\n")
	fid.Write(text)
	fid.Close()

	fid, err = fsys.Open("/2/body", plan9.OREAD)
	if err != nil {
		t.Errorf("Failed to open /2/body: %v", err)
	}
	buf := make([]byte, len(text))
	_, err = fid.ReadFull(buf)
	if err != nil {
		t.Errorf("Failed to read back body: %v", err)
	}
	if string(buf) != string(text) {
		t.Errorf("Corrupted body readback: %v", buf)
	}
	fid.Close()

	fid, err = fsys.Open("/2/addr", plan9.OWRITE)
	if err != nil {
		t.Errorf("Failed to open /2/addr: %v", err)
	}
	fid.Write([]byte("#5"))
	fid.Close()

	// test insertion
	fid, err = fsys.Open("/2/data", plan9.OWRITE)
	if err != nil {
		t.Errorf("Failed to open /2/data: %v", err)
	}
	fid.Write([]byte("insertion"))
	fid.Close()

	fid, err = fsys.Open("/2/body", plan9.OREAD)
	if err != nil {
		t.Errorf("Failed to open /2/body: %v", err)
	}
	text = append(text[0:5], append([]byte("insertion"), text[5:]...)...)
	buf = make([]byte, len(text))
	_, err = fid.ReadFull(buf)
	if err != nil {
		t.Errorf("Failed to read back body: %v", err)
	}
	if string(buf) != string(text) {
		t.Errorf("Corrupted body readback: %v instead of %v", string(buf), string(text))
	}
	fid.Close()

	// Delete the window
	fid, err = fsys.Open("/2/ctl", plan9.OWRITE)
	if err != nil {
		t.Errorf("Failed to open /2/ctl: %v", err)
	}
	fid.Write([]byte("delete"))
	fid.Close()

	// Make sure it's gone from the directory
	fid, err = fsys.Open("/1", plan9.OREAD)
	if err != nil {
		t.Errorf("Failed to walk to /1: %v", err)
	}
	dirs, err := fid.Dirread()
	if err != nil {
		t.Errorf("Failed to open/: %v", err)
	}
	for _, d := range dirs {
		if d.Name == "2" {
			t.Errorf("delete didn't remove /2")
		}
	}
	fid.Close()
}

func TestFSysAddr(t *testing.T) {
	a := startAcme(t)
	defer a.Cleanup()
	tfs := tFsys{t, a.fsys}

	//Add some known text
	text := `
This is a short block
Of text crafted
Just for this 
Occasion
`
	reportchan, exitchan := tfs.startlog()
	defer close(exitchan)

	tfs.Write("/new/body", text)

	op := <-reportchan
	for strings.Index(op, "focus") != -1 {
		op = <-reportchan
	}
	if strings.Index(op, "new") == -1 {
		t.Fatalf("Didn't get report of window creation.")
	}

	id := strings.SplitN(op, " ", 2)[0]
	//	t.Errorf("New window is %v", id)

	winname := "/" + id

	// Addr is not persistent once you close it, so you need
	// to read any desired changes with the same opening.
	fid, err := a.fsys.Open(winname+"/addr", plan9.OREAD|plan9.OWRITE)
	if err != nil {
		t.Fatalf("Failed to open %s/addr", winname)
	}
	// TODO(flux): Should table drive this and add a pile more cases.
	fid.Write([]byte("1,2"))
	var buf [8192]byte
	n, err := fid.Read(buf[:])
	if err != nil {
		t.Fatalf("Failed to read %s/addr", winname)
	}
	var q0, q1 int
	fmt.Sscanf(string(buf[:n]), "%d %d", &q0, &q1)
	if q0 != 0 || q1 != 23 {
		t.Errorf("Expected range of 0..23 retured.  Got %d-%d.", q0, q1)
	}
	fid.Close()
}

type tFsys struct {
	t    *testing.T
	fsys *client.Fsys
}

func (tfs tFsys) startlog() (rc chan string, exit chan struct{}) {
	rc = make(chan string)
	exit = make(chan struct{})
	fid, err := tfs.fsys.Open("/log", plan9.OREAD)
	if err != nil {
		tfs.t.Errorf("Failed to open acme/log: %v", err)
	}

	go func() {
		var buf [1024]byte
		for {
			n, err := fid.Read(buf[:])
			if err != nil {
				return
			}
			rc <- string(buf[0:n])
		}
	}()
	go func() {
		<-exit
		fid.Close()
	}()
	return rc, exit
}

func (tfs tFsys) Read(file string) (s string) {
	fid, err := tfs.fsys.Open(file, plan9.OREAD)
	if err != nil {
		tfs.t.Errorf("Failed to open %s: %v", file, err)
	}
	var buf [8192]byte
	n, err := fid.Read(buf[:])
	if err != nil {
		tfs.t.Errorf("Failed to write %s: %v", file, err)
	}
	fid.Close()
	return string(buf[:n])
}

func (tfs tFsys) Write(file, s string) {
	fid, err := tfs.fsys.Open(file, plan9.OWRITE)
	if err != nil {
		tfs.t.Errorf("Failed to open %s: %v", file, err)
	}
	_, err = fid.Write([]byte(s))
	if err != nil {
		tfs.t.Errorf("Failed to write %s: %v", file, err)
	}
	fid.Close()
}
