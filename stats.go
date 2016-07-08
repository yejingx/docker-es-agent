package main

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/alecthomas/log4go"
	"github.com/fsouza/go-dockerclient"
)

var (
	checking    map[string]bool
	logger      log4go.Logger
	client      *http.Client
	loggerAddr  string
	loggerIndex string
)

func sendMetrix(metrix map[string]interface{}) {
	var err error
	var b []byte
	var req *http.Request
	var resp *http.Response

	metrix["@timestamp"] = time.Now().Unix() * 1000

	b, err = json.Marshal(metrix)
	if err != nil {
		logger.Error("json.Marshal failed, %s", err.Error())
		return
	}

	logger.Debug(string(b))

	index := loggerIndex + "-" + time.Now().Format("2006.01.02")
	url := "http://" + loggerAddr + "/" + index + "/containers"
	req, err = http.NewRequest("POST", url, bytes.NewReader(b))
	if err != nil {
		logger.Error("NewRequest failed, %s", err.Error())
		return
	}

	resp, err = client.Do(req)
	if err != nil {
		logger.Error("Request failed, %s", err.Error())
		return
	}

	logger.Debug("POST metrix to %s returned %d", url, resp.StatusCode)

	io.Copy(ioutil.Discard, resp.Body)
	resp.Body.Close()
}

func statsContainer(client *docker.Client, host, name, cID, appID string) {
	errC := make(chan error, 1)
	statsC := make(chan *docker.Stats)
	done := make(chan bool)

	go func() {
		errC <- client.Stats(docker.StatsOptions{
			ID: cID, Stats: statsC, Stream: true, Done: done,
		})
		close(errC)
	}()

	for {
		stats, ok := <-statsC
		if !ok {
			break
		}

		cpuUsage := stats.CPUStats.CPUUsage.TotalUsage
		systemUsage := stats.CPUStats.SystemCPUUsage

		cpuPercent := 0.0
		cpuDelta := float64(cpuUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
		systemDelta := float64(systemUsage - stats.PreCPUStats.SystemCPUUsage)

		if cpuDelta > 0.0 && systemDelta > 0.0 {
			cpuPercent = (cpuDelta / systemDelta) * float64(len(stats.CPUStats.CPUUsage.PercpuUsage)) * 100.0
		}

		memPercent := float64(stats.MemoryStats.Usage) / float64(stats.MemoryStats.Limit) * 100.0

		sendMetrix(map[string]interface{}{
			"host":        host,
			"cpuPercent":  uint64(cpuPercent),
			"memUsage":    stats.MemoryStats.Usage,
			"memLimit":    stats.MemoryStats.Limit,
			"maxMemUsage": stats.MemoryStats.MaxUsage,
			"memPercent":  uint64(memPercent),
			"name":        name,
			"cID":         cID,
			"appID":       appID,
		})
	}

	delete(checking, cID)

	logger.Debug("stop checking container: %s, name: %s", cID, name)

	if err := <-errC; err != nil {
		logger.Error("get %s stats error, %s", cID, err)
		return
	}
}

func checkConteiners() {
	endpoint := "unix:///var/run/docker.sock"
	client, newErr := docker.NewClient(endpoint)
	if newErr != nil {
		logger.Error("new docker client err, ", newErr.Error())
		return
	}

	opts := docker.ListContainersOptions{
		Filters: map[string][]string{"status": {"running"}},
	}
	containers, listErr := client.ListContainers(opts)
	if listErr != nil {
		logger.Error("list containers err, ", listErr.Error())
		return
	}

	for _, cnt := range containers {
		cID := cnt.ID
		if chk, exists := checking[cID]; exists {
			if chk {
				logger.Debug("container %s already in checking", cID)
			} else {
				logger.Debug("container %s already in skiping", cID)
			}
			continue
		}

		container, err := client.InspectContainer(cID)
		if err != nil {
			logger.Error("inspect container %s error, %s", cID, err.Error())
			continue
		}

		host := "-"
		appID := "-"
		for _, env := range container.Config.Env {
			if idx := strings.Index(env, "MARATHON_APP_ID="); idx == 0 {
				appID = strings.Trim(env[len("MARATHON_APP_ID="):], "/ ")
			} else if idx := strings.Index(env, "HOST="); idx == 0 {
				host = env[len("HOST="):]
			}
		}

		logger.Debug("checking container: %s, name: %s", cID, container.Name)

		go statsContainer(client, host, container.Name, cID, appID)

		checking[cID] = true
	}
}

func main() {
	runtime.GOMAXPROCS(1)

	loglevel := log4go.INFO
	if os.Getenv("LOG_LEVEL") == "debug" {
		loglevel = log4go.DEBUG
	}

	logger = make(log4go.Logger)
	w := log4go.NewConsoleLogWriter()
	w.SetFormat("[%D %T] [%L] %M")
	logger.AddFilter("stdout", loglevel, w)

	checking = make(map[string]bool)

	loggerAddr = os.Getenv("LOGGER_ADDR")
	if loggerAddr == "" {
		logger.Critical("no logger address found")
		logger.Close()
		return
	}

	loggerIndex = os.Getenv("LOGGER_INDEX")
	if loggerIndex == "" {
		loggerIndex = "logstash-docker"
	}

	client = &http.Client{}

	logger.Info("setup statsd client to %s", loggerAddr)

	for {
		logger.Debug("listing all containers")

		checkConteiners()
		time.Sleep(5000 * time.Millisecond)
	}
}
