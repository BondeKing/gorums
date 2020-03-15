// Code generated by protoc-gen-gorums. DO NOT EDIT.
// Source files can be found in: ./cmd/protoc-gen-gorums/dev

package internalgorums

// pkgIdentMap maps from package name to one of the package's identifiers.
// These identifiers are used by the Gorums protoc plugin to generate
// appropriate import statements.
var pkgIdentMap = map[string]string{
	"bytes":                  "Buffer",
	"context":                "Background",
	"encoding/binary":        "LittleEndian",
	"fmt":                    "Errorf",
	"golang.org/x/net/trace": "EventLog",
	"google.golang.org/grpc": "ClientConn",
	"hash/fnv":               "New32a",
	"io":                     "WriteString",
	"log":                    "Logger",
	"net":                    "ResolveTCPAddr",
	"sort":                   "Sort",
	"strconv":                "Atoi",
	"strings":                "Join",
	"sync":                   "Mutex",
	"time":                   "Duration",
}

var staticCode = `// A Configuration represents a static set of nodes on which quorum remote
// procedure calls may be invoked.
type Configuration struct {
	id    uint32
	nodes []*Node
	n     int
	mgr   *Manager
	qspec QuorumSpec
	errs  chan GRPCError
}

// ID reports the identifier for the configuration.
func (c *Configuration) ID() uint32 {
	return c.id
}

// NodeIDs returns a slice containing the local ids of all the nodes in the
// configuration. IDs are returned in the same order as they were provided in
// the creation of the Configuration.
func (c *Configuration) NodeIDs() []uint32 {
	ids := make([]uint32, len(c.nodes))
	for i, node := range c.nodes {
		ids[i] = node.ID()
	}
	return ids
}

// Nodes returns a slice of each available node. IDs are returned in the same
// order as they were provided in the creation of the Configuration.
func (c *Configuration) Nodes() []*Node {
	return c.nodes
}

// Size returns the number of nodes in the configuration.
func (c *Configuration) Size() int {
	return c.n
}

func (c *Configuration) String() string {
	return fmt.Sprintf("configuration %d", c.id)
}

func (c *Configuration) tstring() string {
	return fmt.Sprintf("config-%d", c.id)
}

// Equal returns a boolean reporting whether a and b represents the same
// configuration.
func Equal(a, b *Configuration) bool { return a.id == b.id }

// SubError returns a channel for listening to individual node errors. Currently
// only a single listener is supported.
func (c *Configuration) SubError() <-chan GRPCError {
	return c.errs
}

// A NodeNotFoundError reports that a specified node could not be found.
type NodeNotFoundError uint32

func (e NodeNotFoundError) Error() string {
	return fmt.Sprintf("node not found: %d", e)
}

// A ConfigNotFoundError reports that a specified configuration could not be
// found.
type ConfigNotFoundError uint32

func (e ConfigNotFoundError) Error() string {
	return fmt.Sprintf("configuration not found: %d", e)
}

// An IllegalConfigError reports that a specified configuration could not be
// created.
type IllegalConfigError string

func (e IllegalConfigError) Error() string {
	return "illegal configuration: " + string(e)
}

// ManagerCreationError returns an error reporting that a Manager could not be
// created due to err.
func ManagerCreationError(err error) error {
	return fmt.Errorf("could not create manager: %s", err.Error())
}

// A QuorumCallError is used to report that a quorum call failed.
type QuorumCallError struct {
	Reason     string
	ReplyCount int
	Errors     []GRPCError
}

func (e QuorumCallError) Error() string {
	var b bytes.Buffer
	b.WriteString("quorum call error: ")
	b.WriteString(e.Reason)
	b.WriteString(fmt.Sprintf(" (errors: %d, replies: %d)", len(e.Errors), e.ReplyCount))
	if len(e.Errors) == 0 {
		return b.String()
	}
	b.WriteString("\ngrpc errors:\n")
	for _, err := range e.Errors {
		b.WriteByte('\t')
		b.WriteString(fmt.Sprintf("node %d: %v", err.NodeID, err.Cause))
		b.WriteByte('\n')
	}
	return b.String()
}

// GRPCError is used to report that a single gRPC call failed.
type GRPCError struct {
	NodeID uint32
	Cause  error
}

func (e GRPCError) Error() string {
	return fmt.Sprintf("node %d: %v", e.NodeID, e.Cause.Error())
}

// LevelNotSet is the zero value level used to indicate that no level (and
// thereby no reply) has been set for a correctable quorum call.
const LevelNotSet = -1

// Manager manages a pool of node configurations on which quorum remote
// procedure calls can be made.
type Manager struct {
	mu       sync.Mutex
	nodes    []*Node
	lookup   map[uint32]*Node
	configs  map[uint32]*Configuration
	eventLog trace.EventLog

	closeOnce sync.Once
	logger    *log.Logger
	opts      managerOptions
}

// NewManager attempts to connect to the given set of node addresses and if
// successful returns a new Manager containing connections to those nodes.
func NewManager(nodeAddrs []string, opts ...ManagerOption) (*Manager, error) {
	if len(nodeAddrs) == 0 {
		return nil, fmt.Errorf("could not create manager: no nodes provided")
	}

	m := &Manager{
		lookup:  make(map[uint32]*Node),
		configs: make(map[uint32]*Configuration),
	}

	for _, opt := range opts {
		opt(&m.opts)
	}

	for _, naddr := range nodeAddrs {
		node, err2 := m.createNode(naddr)
		if err2 != nil {
			return nil, ManagerCreationError(err2)
		}
		m.lookup[node.id] = node
		m.nodes = append(m.nodes, node)
	}

	if m.opts.trace {
		title := strings.Join(nodeAddrs, ",")
		m.eventLog = trace.NewEventLog("gorums.Manager", title)
	}

	err := m.connectAll()
	if err != nil {
		return nil, ManagerCreationError(err)
	}

	if m.opts.logger != nil {
		m.logger = m.opts.logger
	}

	if m.eventLog != nil {
		m.eventLog.Printf("ready")
	}

	return m, nil
}

func (m *Manager) createNode(addr string) (*Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("create node %s error: %v", addr, err)
	}

	h := fnv.New32a()
	_, _ = h.Write([]byte(tcpAddr.String()))
	id := h.Sum32()

	if _, found := m.lookup[id]; found {
		return nil, fmt.Errorf("create node %s error: node already exists", addr)
	}

	node := &Node{
		id:      id,
		addr:    tcpAddr.String(),
		latency: -1 * time.Second,
	}

	return node, nil
}

func (m *Manager) connectAll() error {
	if m.opts.noConnect {
		return nil
	}

	if m.eventLog != nil {
		m.eventLog.Printf("connecting")
	}

	for _, node := range m.nodes {
		err := node.connect(m.opts)
		if err != nil {
			if m.eventLog != nil {
				m.eventLog.Errorf("connect failed, error connecting to node %s, error: %v", node.addr, err)
			}
			return fmt.Errorf("connect node %s error: %v", node.addr, err)
		}
	}
	return nil
}

func (m *Manager) closeNodeConns() {
	for _, node := range m.nodes {
		err := node.close()
		if err == nil {
			continue
		}
		if m.logger != nil {
			m.logger.Printf("node %d: error closing: %v", node.id, err)
		}
	}
}

// Close closes all node connections and any client streams.
func (m *Manager) Close() {
	m.closeOnce.Do(func() {
		if m.eventLog != nil {
			m.eventLog.Printf("closing")
		}
		m.closeNodeConns()
	})
}

// NodeIDs returns the identifier of each available node. IDs are returned in
// the same order as they were provided in the creation of the Manager.
func (m *Manager) NodeIDs() []uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]uint32, 0, len(m.nodes))
	for _, node := range m.nodes {
		ids = append(ids, node.ID())
	}
	return ids
}

// Node returns the node with the given identifier if present.
func (m *Manager) Node(id uint32) (node *Node, found bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	node, found = m.lookup[id]
	return node, found
}

// Nodes returns a slice of each available node. IDs are returned in the same
// order as they were provided in the creation of the Manager.
func (m *Manager) Nodes() []*Node {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nodes
}

// ConfigurationIDs returns the identifier of each available
// configuration.
func (m *Manager) ConfigurationIDs() []uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]uint32, 0, len(m.configs))
	for id := range m.configs {
		ids = append(ids, id)
	}
	return ids
}

// Configuration returns the configuration with the given global
// identifier if present.
func (m *Manager) Configuration(id uint32) (config *Configuration, found bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	config, found = m.configs[id]
	return config, found
}

// Configurations returns a slice of each available configuration.
func (m *Manager) Configurations() []*Configuration {
	m.mu.Lock()
	defer m.mu.Unlock()
	configs := make([]*Configuration, 0, len(m.configs))
	for _, conf := range m.configs {
		configs = append(configs, conf)
	}
	return configs
}

// Size returns the number of nodes and configurations in the Manager.
func (m *Manager) Size() (nodes, configs int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.nodes), len(m.configs)
}

// AddNode attempts to dial to the provide node address. The node is
// added to the Manager's pool of nodes if a connection was established.
func (m *Manager) AddNode(addr string) error {
	panic("not implemented")
}

// NewConfiguration returns a new configuration given quorum specification and
// a timeout.
func (m *Manager) NewConfiguration(ids []uint32, qspec QuorumSpec) (*Configuration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(ids) == 0 {
		return nil, IllegalConfigError("need at least one node")
	}

	var cnodes []*Node
	unique := make(map[uint32]struct{})
	var deduped []uint32
	for _, nid := range ids {
		// Ensure that identical ids are only counted once.
		if _, duplicate := unique[nid]; duplicate {
			continue
		}
		unique[nid] = struct{}{}
		deduped = append(deduped, nid)

		node, found := m.lookup[nid]
		if !found {
			return nil, NodeNotFoundError(nid)
		}
		cnodes = append(cnodes, node)
	}

	// Node ids are sorted ensure a globally consistent configuration id.
	sort.Sort(idSlice(deduped))

	h := fnv.New32a()
	for _, id := range deduped {
		binary.Write(h, binary.LittleEndian, id)
	}
	cid := h.Sum32()

	conf, found := m.configs[cid]
	if found {
		return conf, nil
	}

	c := &Configuration{
		id:    cid,
		nodes: cnodes,
		n:     len(cnodes),
		mgr:   m,
		qspec: qspec,
	}
	m.configs[cid] = c

	return c, nil
}

type idSlice []uint32

func (p idSlice) Len() int { return len(p) }

func (p idSlice) Less(i, j int) bool { return p[i] < p[j] }

func (p idSlice) Swap(i, j int) { p[i], p[j] = p[j], p[i] }

const nilAngleString = "<nil>"

// Node encapsulates the state of a node on which a remote procedure call
// can be performed.
type Node struct {
	// Only assigned at creation.
	id      uint32
	addr    string
	conn    *grpc.ClientConn
	mu      sync.Mutex
	lastErr error
	latency time.Duration
	// embed generated nodeServices
	nodeServices
}

// connect to this node to facilitate gRPC calls and optionally client streams.
func (n *Node) connect(opts managerOptions) error {
	var err error
	ctx, cancel := context.WithTimeout(context.Background(), opts.nodeDialTimeout)
	defer cancel()
	n.conn, err = grpc.DialContext(ctx, n.addr, opts.grpcDialOpts...)
	if err != nil {
		return fmt.Errorf("dialing node failed: %w", err)
	}
	return n.connectStream(ctx) // call generated method
}

// close this node for further calls and optionally stream.
func (n *Node) close() error {
	if err := n.conn.Close(); err != nil {
		return fmt.Errorf("%d: conn close error: %w", n.id, err)
	}
	return n.closeStream() // call generated method
}

// ID returns the ID of n.
func (n *Node) ID() uint32 {
	if n != nil {
		return n.id
	}
	return 0
}

// Address returns network address of n.
func (n *Node) Address() string {
	if n != nil {
		return n.addr
	}
	return nilAngleString
}

// Port returns network port of n.
func (n *Node) Port() string {
	if n != nil {
		_, port, _ := net.SplitHostPort(n.addr)
		return port
	}
	return nilAngleString
}

func (n *Node) String() string {
	if n != nil {
		return fmt.Sprintf("addr: %s", n.addr)
	}
	return nilAngleString
}

// FullString returns a more descriptive string representation of n that
// includes id, network address and latency information.
func (n *Node) FullString() string {
	if n != nil {
		n.mu.Lock()
		defer n.mu.Unlock()
		return fmt.Sprintf(
			"node %d | addr: %s | latency: %v",
			n.id, n.addr, n.latency,
		)
	}
	return nilAngleString
}

func (n *Node) setLastErr(err error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.lastErr = err
}

// LastErr returns the last error encountered (if any) when invoking a remote
// procedure call on this node.
func (n *Node) LastErr() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.lastErr
}

func (n *Node) setLatency(lat time.Duration) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.latency = lat
}

// Latency returns the latency of the last successful remote procedure call
// made to this node.
func (n *Node) Latency() time.Duration {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.latency
}

type lessFunc func(n1, n2 *Node) bool

// MultiSorter implements the Sort interface, sorting the nodes within.
type MultiSorter struct {
	nodes []*Node
	less  []lessFunc
}

// Sort sorts the argument slice according to the less functions passed to
// OrderedBy.
func (ms *MultiSorter) Sort(nodes []*Node) {
	ms.nodes = nodes
	sort.Sort(ms)
}

// OrderedBy returns a Sorter that sorts using the less functions, in order.
// Call its Sort method to sort the data.
func OrderedBy(less ...lessFunc) *MultiSorter {
	return &MultiSorter{
		less: less,
	}
}

// Len is part of sort.Interface.
func (ms *MultiSorter) Len() int {
	return len(ms.nodes)
}

// Swap is part of sort.Interface.
func (ms *MultiSorter) Swap(i, j int) {
	ms.nodes[i], ms.nodes[j] = ms.nodes[j], ms.nodes[i]
}

// Less is part of sort.Interface. It is implemented by looping along the
// less functions until it finds a comparison that is either Less or
// !Less. Note that it can call the less functions twice per call. We
// could change the functions to return -1, 0, 1 and reduce the
// number of calls for greater efficiency: an exercise for the reader.
func (ms *MultiSorter) Less(i, j int) bool {
	p, q := ms.nodes[i], ms.nodes[j]
	// Try all but the last comparison.
	var k int
	for k = 0; k < len(ms.less)-1; k++ {
		less := ms.less[k]
		switch {
		case less(p, q):
			// p < q, so we have a decision.
			return true
		case less(q, p):
			// p > q, so we have a decision.
			return false
		}
		// p == q; try the next comparison.
	}
	// All comparisons to here said "equal", so just return whatever
	// the final comparison reports.
	return ms.less[k](p, q)
}

// ID sorts nodes by their identifier in increasing order.
var ID = func(n1, n2 *Node) bool {
	return n1.id < n2.id
}

// Port sorts nodes by their port number in increasing order.
// Warning: This function may be removed in the future.
var Port = func(n1, n2 *Node) bool {
	p1, _ := strconv.Atoi(n1.Port())
	p2, _ := strconv.Atoi(n2.Port())
	return p1 < p2
}

// Latency sorts nodes by latency in increasing order. Latencies less then
// zero (sentinel value) are considered greater than any positive latency.
var Latency = func(n1, n2 *Node) bool {
	if n1.latency < 0 {
		return false
	}
	return n1.latency < n2.latency

}

// Error sorts nodes by their LastErr() status in increasing order. A
// node with LastErr() != nil is larger than a node with LastErr() == nil.
var Error = func(n1, n2 *Node) bool {
	if n1.lastErr != nil && n2.lastErr == nil {
		return false
	}
	return true
}

type managerOptions struct {
	grpcDialOpts    []grpc.DialOption
	nodeDialTimeout time.Duration
	logger          *log.Logger
	noConnect       bool
	trace           bool
}

// ManagerOption provides a way to set different options on a new Manager.
type ManagerOption func(*managerOptions)

// WithDialTimeout returns a ManagerOption which is used to set the dial
// context timeout to be used when initially connecting to each node in its pool.
func WithDialTimeout(timeout time.Duration) ManagerOption {
	return func(o *managerOptions) {
		o.nodeDialTimeout = timeout
	}
}

// WithGrpcDialOptions returns a ManagerOption which sets any gRPC dial options
// the Manager should use when initially connecting to each node in its pool.
func WithGrpcDialOptions(opts ...grpc.DialOption) ManagerOption {
	return func(o *managerOptions) {
		o.grpcDialOpts = opts
	}
}

// WithLogger returns a ManagerOption which sets an optional error logger for
// the Manager.
func WithLogger(logger *log.Logger) ManagerOption {
	return func(o *managerOptions) {
		o.logger = logger
	}
}

// WithNoConnect returns a ManagerOption which instructs the Manager not to
// connect to any of its nodes. Mainly used for testing purposes.
func WithNoConnect() ManagerOption {
	return func(o *managerOptions) {
		o.noConnect = true
	}
}

// WithTracing controls whether to trace qourum calls for this Manager instance
// using the golang.org/x/net/trace package. Tracing is currently only supported
// for regular quorum calls.
func WithTracing() ManagerOption {
	return func(o *managerOptions) {
		o.trace = true
	}
}

type traceInfo struct {
	trace.Trace
	firstLine firstLine
}

type firstLine struct {
	deadline time.Duration
	cid      uint32
}

func (f *firstLine) String() string {
	var line bytes.Buffer
	io.WriteString(&line, "QC: to config")
	fmt.Fprintf(&line, "%v deadline:", f.cid)
	if f.deadline != 0 {
		fmt.Fprint(&line, f.deadline)
	} else {
		io.WriteString(&line, "none")
	}
	return line.String()
}

type payload struct {
	sent bool
	id   uint32
	msg  interface{}
}

func (p payload) String() string {
	if p.sent {
		return fmt.Sprintf("sent: %v", p.msg)
	}
	return fmt.Sprintf("recv from %d: %v", p.id, p.msg)
}

type qcresult struct {
	ids   []uint32
	reply interface{}
	err   error
}

func (q qcresult) String() string {
	var out bytes.Buffer
	io.WriteString(&out, "recv QC reply: ")
	fmt.Fprintf(&out, "ids: %v, ", q.ids)
	fmt.Fprintf(&out, "reply: %v ", q.reply)
	if q.err != nil {
		fmt.Fprintf(&out, ", error: %v", q.err)
	}
	return out.String()
}

func appendIfNotPresent(set []uint32, x uint32) []uint32 {
	for _, y := range set {
		if y == x {
			return set
		}
	}
	return append(set, x)
}

`
