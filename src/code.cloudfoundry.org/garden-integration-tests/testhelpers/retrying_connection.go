package testhelpers

import (
	"io"
	"time"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/garden/client/connection"
)

type RetryingConnection struct {
	Connection connection.Connection
}

func (c *RetryingConnection) Ping() error {
	return c.Connection.Ping()
}

func (c *RetryingConnection) Capacity() (garden.Capacity, error) {
	return c.Connection.Capacity()
}

func (c *RetryingConnection) Create(spec garden.ContainerSpec) (string, error) {
	return c.Connection.Create(spec)
}
func (c *RetryingConnection) List(properties garden.Properties) ([]string, error) {
	return c.Connection.List(properties)
}

func (c *RetryingConnection) Destroy(handle string) error {
	return c.Connection.Destroy(handle)
}

func (c *RetryingConnection) Stop(handle string, kill bool) error {
	return c.Connection.Stop(handle, kill)
}

func (c *RetryingConnection) Info(handle string) (garden.ContainerInfo, error) {
	return c.Connection.Info(handle)
}

func (c *RetryingConnection) BulkInfo(handles []string) (map[string]garden.ContainerInfoEntry, error) {
	return c.Connection.BulkInfo(handles)
}
func (c *RetryingConnection) BulkMetrics(handles []string) (map[string]garden.ContainerMetricsEntry, error) {
	return c.Connection.BulkMetrics(handles)
}

func (c *RetryingConnection) StreamIn(handle string, spec garden.StreamInSpec) error {
	return c.Connection.StreamIn(handle, spec)
}
func (c *RetryingConnection) StreamOut(handle string, spec garden.StreamOutSpec) (io.ReadCloser, error) {
	return c.Connection.StreamOut(handle, spec)
}

func (c *RetryingConnection) CurrentBandwidthLimits(handle string) (garden.BandwidthLimits, error) {
	return c.Connection.CurrentBandwidthLimits(handle)
}
func (c *RetryingConnection) CurrentCPULimits(handle string) (garden.CPULimits, error) {
	return c.Connection.CurrentCPULimits(handle)
}
func (c *RetryingConnection) CurrentDiskLimits(handle string) (garden.DiskLimits, error) {
	return c.Connection.CurrentDiskLimits(handle)
}
func (c *RetryingConnection) CurrentMemoryLimits(handle string) (garden.MemoryLimits, error) {
	return c.Connection.CurrentMemoryLimits(handle)
}

func (c *RetryingConnection) Run(handle string, spec garden.ProcessSpec, io garden.ProcessIO) (garden.Process, error) {
	process, err := c.Connection.Run(handle, spec, io)
	if err != nil {
		return nil, err
	}
	return &RetryingProcess{Process: process}, nil
}

func (c *RetryingConnection) Attach(handle string, processID string, io garden.ProcessIO) (garden.Process, error) {
	return c.Connection.Attach(handle, processID, io)
}

func (c *RetryingConnection) NetIn(handle string, hostPort, containerPort uint32) (uint32, uint32, error) {
	return c.Connection.NetIn(handle, hostPort, containerPort)
}

func (c *RetryingConnection) NetOut(handle string, rule garden.NetOutRule) error {
	return c.Connection.NetOut(handle, rule)
}

func (c *RetryingConnection) BulkNetOut(handle string, rules []garden.NetOutRule) error {
	return c.Connection.BulkNetOut(handle, rules)
}

func (c *RetryingConnection) SetGraceTime(handle string, graceTime time.Duration) error {
	return c.Connection.SetGraceTime(handle, graceTime)
}

func (c *RetryingConnection) Properties(handle string) (garden.Properties, error) {
	return c.Connection.Properties(handle)
}

func (c *RetryingConnection) Property(handle string, name string) (string, error) {
	return c.Connection.Property(handle, name)
}

func (c *RetryingConnection) SetProperty(handle string, name string, value string) error {
	return c.Connection.SetProperty(handle, name, value)
}

func (c *RetryingConnection) RemoveProperty(handle string, name string) error {
	return c.Connection.RemoveProperty(handle, name)
}

func (c *RetryingConnection) Metrics(handle string) (garden.Metrics, error) {
	return c.Connection.Metrics(handle)
}
