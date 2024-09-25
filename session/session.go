package session

import (
	"crypto/rand"
	"log"
	"math/big"
	"net"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/imdario/mergo"
)

// Caps - user capabilities
type Caps struct {
	Name                  string            `json:"browserName,omitempty"`
	DeviceName            string            `json:"deviceName,omitempty"`
	Version               string            `json:"version,omitempty"`
	W3CVersion            string            `json:"browserVersion,omitempty"`
	Platform              string            `json:"platform,omitempty"`
	W3CPlatform           string            `json:"platformName,omitempty"`
	W3CDeviceName         string            `json:"appium:deviceName,omitempty"`
	ScreenResolution      string            `json:"screenResolution,omitempty"`
	Skin                  string            `json:"skin,omitempty"`
	VNC                   bool              `json:"enableVNC,omitempty"`
	Video                 bool              `json:"enableVideo,omitempty"`
	Log                   bool              `json:"enableLog,omitempty"`
	VideoName             string            `json:"videoName,omitempty"`
	VideoScreenSize       string            `json:"videoScreenSize,omitempty"`
	VideoFrameRate        uint16            `json:"videoFrameRate,omitempty"`
	VideoCodec            string            `json:"videoCodec,omitempty"`
	LogName               string            `json:"logName,omitempty"`
	TestName              string            `json:"name,omitempty"`
	TimeZone              string            `json:"timeZone,omitempty"`
	ContainerHostname     string            `json:"containerHostname,omitempty"`
	Env                   []string          `json:"env,omitempty"`
	ApplicationContainers []string          `json:"applicationContainers,omitempty"`
	AdditionalNetworks    []string          `json:"additionalNetworks,omitempty"`
	HostsEntries          []string          `json:"hostsEntries,omitempty"`
	DNSServers            []string          `json:"dnsServers,omitempty"`
	Labels                map[string]string `json:"labels,omitempty"`
	SessionTimeout        string            `json:"sessionTimeout,omitempty"`
	S3KeyPattern          string            `json:"s3KeyPattern,omitempty"`
	ExtensionCapabilities *Caps             `json:"selenoid:options,omitempty"`
}

func (c *Caps) ProcessExtensionCapabilities() {
	if c.W3CVersion != "" {
		c.Version = c.W3CVersion
	}
	if c.W3CPlatform != "" {
		c.Platform = c.W3CPlatform
	}
	if c.W3CDeviceName != "" {
		c.DeviceName = c.W3CDeviceName
	}

	if c.ExtensionCapabilities != nil {
		mergo.Merge(c, *c.ExtensionCapabilities, mergo.WithOverride) //We probably need to handle returned error
	}
}

func (c *Caps) BrowserName() string {
	browserName := c.Name
	if browserName != "" {
		return browserName
	}
	if c.DeviceName != "" {
		return c.DeviceName
	}
	return c.W3CDeviceName
}

// Container - container information
type Container struct {
	ID        string            `json:"id"`
	IPAddress string            `json:"ip"`
	Ports     map[string]string `json:"exposedPorts,omitempty"`
}

// Session - holds session info
type Session struct {
	Quota     string
	Caps      Caps
	URL       *url.URL
	Container *Container
	HostPort  HostPort
	Origin    string
	Cancel    func()
	Timeout   time.Duration
	TimeoutCh chan struct{}
	Started   time.Time
	Lock      sync.Mutex
}

// HostPort - hold host-port values for all forwarded ports
type HostPort struct {
	Selenium   string
	Fileserver string
	Clipboard  string
	VNC        string
	Devtools   string
}

// Map - session uuid to sessions mapping
type Map struct {
	m map[string]*Session
	l sync.RWMutex
}

// NewMap - create session map
func NewMap() *Map {
	return &Map{m: make(map[string]*Session)}
}

// Get - synchronous get session
func (m *Map) Get(k string) (*Session, bool) {
	m.l.RLock()
	defer m.l.RUnlock()
	s, ok := m.m[k]
	return s, ok
}

// Put - synchronous put session
func (m *Map) Put(k string, v *Session) {
	m.l.Lock()
	defer m.l.Unlock()
	m.m[k] = v
}

// Remove - synchronous remove session
func (m *Map) Remove(k string) {
	m.l.Lock()
	defer m.l.Unlock()
	delete(m.m, k)
}

// Each - synchronous iterate through sessions
func (m *Map) Each(fn func(k string, v *Session)) {
	m.l.RLock()
	defer m.l.RUnlock()
	for k, v := range m.m {
		fn(k, v)
	}
}

// Len - get total number of sessions
func (m *Map) Len() int {
	m.l.RLock()
	defer m.l.RUnlock()
	return len(m.m)
}

// Metadata - session metadata saved to file
type Metadata struct {
	ID           string    `json:"id"`
	Capabilities Caps      `json:"capabilities"`
	Started      time.Time `json:"started"`
	Finished     time.Time `json:"finished"`
}

type PortsStorage struct {
	m map[int64]interface{}
	l sync.RWMutex
}

var Ports = &PortsStorage{m: make(map[int64]interface{})}

func (dvp *PortsStorage) remove(k int64) {
	dvp.l.Lock()
	defer dvp.l.Unlock()
	delete(dvp.m, k)
}

func (dvp *PortsStorage) GetFreePort(portOffset int) int64 {
	dvp.l.Lock()
	defer dvp.l.Unlock()
	for i := 0; i < 10000; i++ {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(5000)))
		var port = n.Int64() + int64(portOffset)
		_, ok := dvp.m[port]
		if ok {
			continue
		} else {
			dial, err := net.DialTimeout("tcp4", "127.0.0.1:"+strconv.Itoa(int(port)), 100*time.Millisecond)
			if err == nil {
				log.Printf("port '%d' is busy", port)
				dial.Close()
				continue
			}
			dvp.m[port] = new(interface{})
			return port
		}
	}
	panic("all ports busy")
}

func (dvp *PortsStorage) ReleasePort(port int64) {
	dvp.remove(port)
	log.Printf("[PORT_NOW_FREE] [%d]", port)
}
