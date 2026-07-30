package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/logrusorgru/aurora"

	"code.cloudfoundry.org/dontpanic/collectors/command"
	"code.cloudfoundry.org/dontpanic/collectors/file"
	"code.cloudfoundry.org/dontpanic/collectors/grootfs"
	"code.cloudfoundry.org/dontpanic/collectors/process"
	"code.cloudfoundry.org/dontpanic/commandrunner"
	"code.cloudfoundry.org/dontpanic/osreporter"
	flags "github.com/jessevdk/go-flags"
)

type Server struct {
	LogLevel string `long:"log-level" default:"info"`
}

type Config struct {
	Server Server `command:"server"`
}

func main() {
	var opts struct {
		SigQUIT bool `long:"sigquit" description:"Send a SIGQUIT to the gdn process"`
	}

	handleFlagErrors(flags.ParseArgs(&opts, os.Args))

	checkIsRoot()
	checkIsNotBpm()
	checkGardenLogLevel()

	reportDir := createReportDir("/var/vcap/data/tmp")
	osReporter := osreporter.New(reportDir, os.Stdout)

	if opts.SigQUIT {
		osReporter.RegisterCollector("Dump gdn goroutines", command.NewDiscardCollector("pkill -QUIT gdn"))
	}

	osReporter.RegisterNoisyCollector("Date", command.NewCollector("date", "date.log"))
	osReporter.RegisterNoisyCollector("Uptime", command.NewCollector("uptime", "uptime.log"))
	osReporter.RegisterNoisyCollector("Garden Version", command.NewCollector("/var/vcap/packages/guardian/bin/gdn -v", "gdn-version.log"))
	osReporter.RegisterNoisyCollector("Hostname", command.NewCollector("hostname", "hostname.log"))
	osReporter.RegisterNoisyCollector("Memory Usage", command.NewCollector("free -mt", "free.log"))
	osReporter.RegisterNoisyCollector("Kernel Details", command.NewCollector("uname -a", "uname.log"))
	osReporter.RegisterNoisyCollector("Monit Summary", command.NewCollector("/var/vcap/bosh/bin/monit summary", "monit-summary.log"))
	osReporter.RegisterNoisyCollector("Number of Open Files", command.NewCollector("lsof 2>/dev/null | wc -l", "num-open-files.log"))
	osReporter.RegisterNoisyCollector("Max Number of Open Files", command.NewCollector("cat /proc/sys/fs/file-max", "file-max.log"))

	osReporter.RegisterCollector("Disk Usage", command.NewCollector("df -h", "df.log"))
	osReporter.RegisterCollector("GrootFS Unprivileged Usage", grootfs.NewUsageCollector("/var/vcap/jobs/garden/config/grootfs_config.yml", commandrunner.CommandRunner{}))
	osReporter.RegisterCollector("GrootFS Privileged Usage", grootfs.NewUsageCollector("/var/vcap/jobs/garden/config/privileged_grootfs_config.yml", commandrunner.CommandRunner{}))
	osReporter.RegisterCollector("List of Open Files", command.NewCollector("lsof", "lsof.log"))
	osReporter.RegisterCollector("Map of Inodes to Paths", command.NewCollector(`find / -fprintf inodes '%i %p\n'; lsof -Fi | grep '^i' | cut -c2- | sort | uniq | xargs -i grep -w ^{} inodes; rm inodes`, "inodes.log"), time.Second*60)
	osReporter.RegisterCollector("Process Information", command.NewCollector("ps -eLo pid,tid,ppid,user:11,comm,state,wchan:35,lstart", "ps-info.log"))
	osReporter.RegisterCollector("Process Tree", command.NewCollector("ps aux --forest", "ps-forest.log"))
	osReporter.RegisterCollector("Kernel Messages", command.NewCollector("dmesg -T", "dmesg.log"))
	osReporter.RegisterCollector("Network Interfaces", command.NewCollector("ifconfig", "ifconfig.log"))
	osReporter.RegisterCollector("IP Tables", command.NewCollector("iptables -L -w", "iptables-L.log"))
	osReporter.RegisterCollector("NAT IP Tables", command.NewCollector("iptables -tnat -L -w", "iptables-tnat.log"))
	osReporter.RegisterCollector("Mount Table", command.NewCollector("cat /proc/$(pidof gdn)/mountinfo", "mountinfo.log"))
	osReporter.RegisterCollector("Garden Depot Contents", command.NewCollector("find /var/vcap/data/garden/depot | sed 's|[^/]*/|- |g'", "depot-contents.log"))
	osReporter.RegisterCollector("XFS Fragmentation", command.NewCollector("xfs_db -r -c frag /var/vcap/data/grootfs/store/unprivileged.backing-store", "xfs-frag.log"))
	osReporter.RegisterCollector("XFS Info", command.NewCollector("xfs_info /var/vcap/data/grootfs/store/unprivileged", "xfs-info.log"))
	osReporter.RegisterCollector("Slabinfo", command.NewCollector("cat /proc/slabinfo", "slabinfo.log"))
	osReporter.RegisterCollector("Meminfo", command.NewCollector("cat /proc/meminfo", "meminfo.log"))
	osReporter.RegisterCollector("IOSTAT -xdm (slow)", command.NewCollector("iostat -x -d -m 5 3", "iostat.log"), time.Second*16)
	osReporter.RegisterCollector("VMSTAT -s", command.NewCollector("vmstat -s", "vmstat-s.log"))
	osReporter.RegisterCollector("VMSTAT -d (slow)", command.NewCollector("vmstat -d 5 3", "vmstat-d.log"), time.Second*16)
	osReporter.RegisterCollector("VMSTAT -a (slow)", command.NewCollector("vmstat -a 5 3", "vmstat-a.log"), time.Second*16)
	osReporter.RegisterCollector("Mass Process Data", process.NewCollector("process-data"))

	osReporter.RegisterCollector("Kernel Log", file.NewCollector("/var/log/kern.log*", "kernel-logs/"))
	osReporter.RegisterCollector("Monit Log", file.NewCollector("/var/vcap/monit/monit.log", "monit.log"))
	osReporter.RegisterCollector("Syslog", file.NewCollector("/var/log/syslog*", "syslogs/"))
	osReporter.RegisterCollector("Garden Config", file.NewDirCollector("/var/vcap/jobs/garden/config", ""))
	osReporter.RegisterCollector("Garden Logs", file.NewDirCollector("/var/vcap/sys/log/garden", ""))
	osReporter.RegisterCollector("Sysstat", file.NewDirCollector("/var/log/sysstat", ""))

	osReporter.RegisterCollector("Garden Containers", command.NewCollector("(curl localhost:7777/containers || curl --no-buffer -XGET --unix-socket /var/vcap/data/garden/garden.sock http://localhost/containers) 2> /dev/null", "garden-containers.log"))
	if isContainerd() {
		osReporter.RegisterCollector("Containerd Init Containers", command.NewCollector(`/var/vcap/packages/containerd/bin/ctr -a /var/vcap/sys/run/containerd/containerd.sock -n garden containers ls 'labels."container-type"==garden-init'`, "containerd/init-containers"))
		osReporter.RegisterCollector("Containerd Pea Containers", command.NewCollector(`/var/vcap/packages/containerd/bin/ctr -a /var/vcap/sys/run/containerd/containerd.sock -n garden containers ls 'labels."container-type"==pea'`, "containerd/pea-containers"))
		osReporter.RegisterCollector("Containerd Tasks", command.NewCollector(`/var/vcap/packages/containerd/bin/ctr -a /var/vcap/sys/run/containerd/containerd.sock -n garden tasks ls`, "containerd/tasks"))
	}

	if err := osReporter.Run(); err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
}

func checkIsRoot() {
	if currentUID := os.Geteuid(); currentUID != 0 {
		fmt.Fprintln(os.Stderr, aurora.Red("Keep Calm and Re-run as Root!").Bold())
		os.Exit(1)
	}
}

func checkIsNotBpm() {
	contents, err := os.ReadFile("/proc/1/cmdline")
	if err != nil {
		fmt.Fprintln(os.Stderr, aurora.Red("Cannot determine if running in bpm: cannot read cmdline").Bold())
		os.Exit(1)
	}

	if bytes.Contains(contents, []byte("garden_start")) {
		fmt.Fprintln(os.Stderr, aurora.Red("Keep Calm and Re-run outside the BPM container!").Bold())
		os.Exit(1)
	}
}

func checkGardenLogLevel() {
	config := Config{Server: Server{}}
	if err := flags.IniParse("/var/vcap/jobs/garden/config/config.ini", &config); err != nil {
		return
	}

	if config.Server.LogLevel == "error" || config.Server.LogLevel == "fatal" {
		fmt.Fprintln(
			os.Stderr, aurora.Yellow(
				"WARNING: Garden log level is set to "+
					config.Server.LogLevel+
					". This usually makes problem diagnosis quite hard. "+
					"Please consider using a log level of 'debug' or 'info' in the future!",
			).Bold())
	}
}

func isContainerd() bool {
	_, err := os.Stat("/var/vcap/sys/run/containerd/containerd.sock")
	return !os.IsNotExist(err)
}

func createReportDir(baseDir string) string {
	hostname, err := os.Hostname()
	if err != nil {
		fmt.Println(aurora.Magenta("could not determine hostname"))
		hostname = "UNKNOWN-HOSTNAME"
	}
	timestamp := time.Now().Format("2006-01-02-15-04-05.000000000")
	reportDir := fmt.Sprintf("os-report-%s-%s", hostname, timestamp)
	path := filepath.Join(baseDir, reportDir)
	if err := os.MkdirAll(path, 0755); err != nil {
		fmt.Fprintln(os.Stderr, aurora.Red(fmt.Sprintf("cannot create report directory %q: %s", path, err.Error())))
	}
	return path
}

func handleFlagErrors(_ []string, err error) {
	flagErr := &flags.Error{}
	if errors.As(err, &flagErr) && flagErr.Type == flags.ErrHelp {
		os.Exit(0)
	}

	if err != nil {
		os.Exit(1)
	}
}
