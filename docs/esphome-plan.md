# ESPHome integration design plan

## Context

We want mgmt to interact with [ESPHome](https://esphome.io/) devices (buttons,
LEDs, sensors, and eventually motors) reactively: mcl functions that stream
live entity state, and resources that enforce desired entity state. A typical
first use case: physical buttons on an ESPHome device advance/reverse an
active LED, with mgmt holding the desired state and repairing any out-of-band
changes.

The architecture:

- An `esphome:endpoint` resource whose **name is a uid**; fields carry the
  connection params plus an `interval` field (0 = native events, >0 = poll
  every N seconds, reconnecting each time).
- Functions in a new `lang/core/net/esphome` package take an `endpoint` param
  (the uid) and return **zero values until the endpoint resource has
  published its connection info** (brokered by the generic Bridge API in
  `engine/local/local.go`).
- A shared util library connection-pools so that any number of
  functions/resources talking to one device share **one real connection**.
- Everything pure golang.

## Protocol background

The ESPHome native API is **protobuf over a persistent TCP connection
(default port 6053)**, framed either plaintext or Noise-NNpsk0 encrypted.
After the client sends one `SubscribeStatesRequest`, the device immediately
sends a snapshot of every entity's state and then **pushes** a state message
whenever any entity changes. It is genuinely event-driven, *not* long
polling. There is **no per-entity read request** in the protocol: state only
arrives via that subscription. Keepalive is `PingRequest`/`PingResponse`.
Entities are discovered with `ListEntitiesRequest` which returns per-domain
responses, each carrying a stable `object_id` (from the device YAML), a
numeric `key` (hash), and a friendly name; commands (`SwitchCommandRequest`,
`NumberCommandRequest`, and so on) address entities by `key`.

Consequences for our design:

- `interval == 0` (native events) is the natural, efficient mode: one
  persistent connection with pushed updates.
- `interval > 0` (polling): **reconnect every cycle** -- connect, handshake,
  subscribe, capture the initial snapshot burst, send any pending commands,
  disconnect, sleep `interval` seconds.
- Auth: the legacy `password`/`ConnectRequest` flow is deprecated and removed
  in ESPHome 2026.1; the Noise PSK (`api: encryption: key:`) is the current
  mechanism. There is no username in the protocol. We still model a
  `password` field for older firmware, but `key` is the primary credential.

## Library analysis

Three candidates were cloned and code-inspected.

### 1. `github.com/richard87/esphome-apiclient` -- use this now

MIT. ~3.6k LOC handwritten + generated protobuf. Actively developed in 2026.

**Quality: sufficient as-is for our POC.** Verified in source:

- Clean layering: `transport/` (plain + Noise via `flynn/noise`, 600-line
  Noise test), `framer`, message-type `router`, `client`, typed `commands`.
- Real test suite: client, keepalive, readloop, commands, entities, noise.
- `EntityRegistry` is `sync.RWMutex`-guarded, typed per domain
  (`SwitchEntity`, `BinarySensorEntity`, ..., each with `ObjectID`, `Key`,
  cached `State` + `MissingState`), with `ByKey`/`ByName` lookups.
- `SubscribeStates` re-registers automatically after reconnect; keepalive
  closes dead connections; reconnect uses exponential backoff.
- Typed command helpers we need day one: `SetSwitch`, `SetNumber`,
  `PressButton`, `SetLight`, `SetFan` (H-bridge motors), `SetCover`, etc.
- Protocol currency: proto includes 2025/2026-era families (Update, Valve,
  WaterHeater, Infrared, VoiceAssistant).
- Library deps are minimal (`flynn/noise`, `google.golang.org/protobuf`;
  `urfave/cli`/`miekg/dns` are only for its CLI).

**Known warts (all tolerable, none blocking):**

- **No `ConnectRequest`/password support** -- Noise-key and open plaintext
  only. Legacy password-auth firmware won't work until patched (~30 lines).
  Acceptable: passwords are removed upstream in ESPHome 2026.1 anyway.
- **Races in its built-in reconnect path** (`c.done` channel is replaced
  unsynchronized; registry `Clear()` mid-use). Mitigation: we disable its
  reconnect (`WithReconnect(0)`) and own the reconnect/poll loop in our util,
  which we need anyway for the interval semantics.
- `go 1.26.1` directive vs mgmt's `go 1.25.7` -- needs a toolchain >= 1.26.1
  or a trivial upstream/fork patch lowering the directive.
- No `ByObjectID` lookup (we iterate or index ourselves in the util).

**Fork/patch recommendation:** consume upstream as-is first. Candidate
upstream PRs (nice, not required): `ByObjectID()`, lower the `go` directive,
optional `ConnectRequest` support. Fork only if the single maintainer is
unresponsive and one of these becomes blocking.

### 2. `github.com/mycontroller-org/esphome_api` -- not recommended

Apache-2.0, stable-but-stale (proto synced Sep 2024, last commit Sep 2024).
Lower level: raw `proto.Message` callback, single reader goroutine + waitMap.
It *does* support legacy password `Login()`. But: no entity registry, no
keepalive automation, no reconnect, `chan bool` stop signalling with a
non-blocking send (reader can outlive `Close`), and its `go.mod` drags in the
whole MyController server module. We would rebuild everything the first
candidate already has. Only relevant if legacy-password support becomes a
hard requirement before upstream/fork patching is feasible.

### 3. `github.com/flavio-fernandes/go-aioesphomeapi` -- the strategic destination, not usable today

Bootstrapped 2026-07-16, docs/governance only -- zero golang implementation
yet (Gate 0 of its roadmap). Its ADRs align with this plan: MIT core,
mgmt-first, Noise-by-default, *"generic device connection sharing within a
process = go-aioesphomeapi plus an MGMT-local registry"*, and the mgmt
adapter explicitly lives in the mgmt repo. Its `go.mod` already pins golang
1.25.7 to match mgmt. Its Milestone 1 (binary_sensor, sensor, switch, fan)
covers exactly our POC surface.

### Portability verdict

Easy to port away later, by construction: our util wraps the wire library
behind a small internal `driver` interface (~8 methods, one adapter file).
Nothing outside `util/esphome/` imports the third-party module. Swapping
richard87 for go-aioesphomeapi later means writing one new adapter file and
changing one constructor. This is also the seam our unit tests use (fake
driver).

**Decision: richard87/esphome-apiclient now, behind the driver seam; migrate
to go-aioesphomeapi when its M1 client contract is real.**

## Architecture

```
mcl: esphome:endpoint "garage" { host => "...", key => "...", interval => 0 }
          |  CheckApply publishes *ConnInfo
          v
  local.BridgeSet("esphome:endpoint", "garage", connInfo)   [engine/local]
          |                                    ^
          |  BridgeWatch/BridgeGet             |  (nil until resource runs
          v                                    |   -> functions return zero)
  functions net.esphome.*("garage", ...)       |
          |                                    |
          v                                    |
  util/esphome pool -- one Session per endpoint uid -- one TCP conn per device
          ^                                    (persistent OR poll-reconnect)
          |
  resources esphome:switch, esphome:number, ... (commands + repair watch)
```

- The **endpoint resource** never holds the device connection itself; it
  validates params, publishes `*ConnInfo` on the bridge (namespace
  `esphome:endpoint`, uid = resource name), and unpublishes on `Cleanup()`.
- The **pool** (package-level, refcounted, keyed by endpoint uid -- modeled
  on `bmcStateReserve` in `engine/resources/bmc.go`) owns one `Session` per
  uid. All functions and entity resources naming that uid share it, so
  exactly one real device connection exists per endpoint.
- Two `esphome:endpoint` resources pointing at the same physical device get
  two connections -- documented, acceptable (ESP devices allow ~3 clients).

## New package: `util/esphome` (package `esphomeutil`)

The complete API surface for the POC:

```go
// ConnInfo is the value the esphome:endpoint resource publishes on the
// bridge. Consumers must treat it as immutable.
type ConnInfo struct {
	Host     string // ip address or hostname
	Port     int    // default 6053
	Key      string // base64 noise psk (preferred)
	Password string // legacy auth; unsupported by the initial driver
	Interval uint32 // 0 = native push events; >0 = poll (reconnect) every N sec
}
func (obj *ConnInfo) Addr() string             // "host:port"
func (obj *ConnInfo) Cmp(info *ConnInfo) error // field-wise compare
func (obj *ConnInfo) Validate() error

// State is an immutable snapshot of one entity's last-known state.
type State struct {
	Domain  string  // "binary_sensor", "sensor", "switch", "number", ...
	Bool    bool    // binary_sensor, switch
	Float   float64 // sensor, number
	Str     string  // text_sensor
	Missing bool    // device reported state as missing/unknown
}

// Package-level refcounted pool (mutex + map, bmc.go pattern).
func SessionReserve(uid string) *Session // get-or-create, refcount++
func (obj *Session) Release()            // refcount--, closes conn at zero

// Session is the shared per-endpoint connection manager.
func (obj *Session) Configure(info *ConnInfo)
	// Idempotent (ConnInfo.Cmp). nil = endpoint unpublished -> disconnect,
	// clear cache, notify. Change -> reconnect with new params. Each
	// consumer calls this whenever its BridgeWatch fires; the session
	// dedupes.
func (obj *Session) Watch(ctx context.Context) (chan struct{}, error)
	// Startup event + one event per state-cache change, connect/disconnect
	// included. Same chan discipline as local.HTTPPool.HTTPWatch.
func (obj *Session) State(objectID string) *State // nil if unknown
func (obj *Session) Connected() bool

// Commands (by object_id; resolved to entity key via the registry).
// In poll mode these wake the loop to connect immediately, send, disconnect.
func (obj *Session) SetSwitch(ctx context.Context, objectID string, on bool) error
func (obj *Session) SetNumber(ctx context.Context, objectID string, value float64) error
func (obj *Session) PressButton(ctx context.Context, objectID string) error

// driver is the internal seam hiding the wire library (richard87 today,
// go-aioesphomeapi later). One adapter file implements it.
type driver interface {
	Connect(ctx context.Context, info *ConnInfo) error
	Close() error
	Entities() []EntityInfo // {Key uint32; ObjectID, Name, Domain string}
	SubscribeStates(fn func(EntityState)) (unsubscribe func(), err error)
	SetSwitch(key uint32, on bool) error
	SetNumber(key uint32, value float32) error
	PressButton(key uint32) error
}
```

**Session internals:** a mainloop goroutine started on first `Configure`.

- `interval == 0`: connect (Noise if `Key != ""`), `ListEntities`,
  `SubscribeStates`; on push -> update `map[objectID]State` cache + notify
  watchers; on error/disconnect -> mark disconnected, notify, retry with
  backoff. The library's own reconnect stays disabled (`WithReconnect(0)`) --
  we own the loop (avoids its racy path).
- `interval > 0`: each cycle: connect, list, subscribe, wait for the initial
  snapshot burst (short settle timeout), update cache, flush pending
  commands, disconnect, sleep interval (or until a command wakes it).
- Notify mechanism: `mutex` + `notify map[chan struct{}]struct{}` with
  buffered-chan coalescing, copied from `HTTPPool` in `engine/local/local.go`.

Files: `util/esphome/esphome.go` (ConnInfo, State, pool),
`util/esphome/session.go` (mainloop, cache, watch), `util/esphome/driver.go`
+ `util/esphome/apiclient.go` (richard87 adapter),
`util/esphome/esphome_test.go` (fake driver tests).

## Resource: `esphome:endpoint` (`engine/resources/esphome.go`)

Registration follows the colon-kind convention (`tftp.go`,
`http_client.go`): `const esphomeKind = "esphome"`, register
`esphomeKind + ":endpoint"`. Anatomy copied from `value.go` +
`http_client.go`:

```go
type EsphomeEndpointRes struct {
	traits.Base

	init *engine.Init

	// Host is the ip address or hostname of the esphome device.
	Host string `lang:"host" yaml:"host"`
	// Port is the native api port. Defaults to 6053.
	Port int `lang:"port" yaml:"port"`
	// Key is the base64 noise encryption key (api: encryption: key:).
	Key string `lang:"key" yaml:"key"`
	// Password is the legacy api password. Deprecated by esphome; prefer
	// Key.
	Password string `lang:"password" yaml:"password"`
	// Interval: 0 = native push events; >0 = poll every N seconds,
	// reconnecting each time.
	Interval uint32 `lang:"interval" yaml:"interval"`
}
```

- `Default()`: `Port: 6053`. `Validate()`: host non-empty, port range,
  interval sanity, key base64/32-byte check, error if both Key and Password.
- `Watch()`: minimal `value.go` shape (startup event + ctx.Done) for the
  POC. (Later: reserve the session and forward its Watch events so device
  connectivity shows as resource events.)
- `CheckApply()`: the `http_client.go` defer-publish pattern -- named
  returns, `defer` a `publishConnInfo(ctx)` helper that guards
  `ctx.Err() != nil`, `BridgeGet`-compares, then
  `obj.init.Local.BridgeSet(ctx, "esphome:endpoint", obj.Name(), connInfo)`.
  State check = "is the published value current?" (checkOK true if
  unchanged; publish only when `apply` -- Init stays read-only).
- `Cleanup()`: `BridgeSet(ctx, ns, obj.Name(), nil)` to unpublish.
- `Cmp()`, `UnmarshalYAML` boilerplate per `value.go`.

## Resources: entity control (`engine/resources/esphome_entities.go`)

POC set (entity-domain names):

- **`esphome:switch`** -- GPIO outputs / relays / LEDs. Name = ESPHome
  `object_id` (overridable via an `Id` field). Fields: `endpoint str` (uid
  of the esphome:endpoint), `state str` ("on"/"off", svc.go-style).
- **`esphome:number`** -- setpoints/speeds (motor speed later). Fields:
  `endpoint str`, `value float`.

Shared lifecycle (both):

- `Init()`: read-only setup only (no state changes in Init).
- `Watch()`: `BridgeWatch(ctx, "esphome:endpoint", obj.Endpoint)` + reserve
  session + `session.Watch(ctx)`; forward events via `obj.init.Event(ctx)`
  -- this is what makes mgmt *repair* out-of-band flips (someone toggles the
  switch in Home Assistant -> state event -> CheckApply flips it back).
- `CheckApply()`: `BridgeGet` conn info; if nil or not connected -> error
  ("endpoint x not available") so the engine retries per retry metaparams.
  Else `Configure(info)`, compare `session.State(objectID)` to desired; if
  different and `apply`, send `SetSwitch`/`SetNumber`.
- `Cleanup()`: `session.Release()`.
- **AutoEdges**: `esphome:endpoint["garage"]` -> `esphome:switch` with
  `endpoint => "garage"` (standard UIDs/AutoEdges pattern), so endpoints
  apply first and entity resources rarely hit the "not available" retry.

Future resources (same skeleton): `esphome:light`, `esphome:fan` (H-bridge
motor direction/speed -- the conveyor use case), `esphome:cover`,
`esphome:button` (press-on-refresh via the refresh trait), `esphome:select`,
`esphome:text`, `esphome:lock`, `esphome:climate`.

## Functions: `lang/core/net/esphome/` (package `corenetesphome`)

Registered so mcl sees `net.esphome.*` -- exactly like
`lang/core/net/http/response.go`:

```go
funcs.ModuleRegister(corenet.ModuleName+"/"+ModuleName, "binary_sensor",
	func() interfaces.Func { return &BinarySensorFunc{} })
```

POC set (entity-domain names; all take `(endpoint str, id str)` where `id`
is the ESPHome `object_id`):

| Function | Signature | Zero value |
|---|---|---|
| `net.esphome.binary_sensor` | `func(endpoint str, id str) bool` | `false` |
| `net.esphome.sensor` | `func(endpoint str, id str) float` | `0.0` |
| `net.esphome.text_sensor` | `func(endpoint str, id str) str` | `""` |
| `net.esphome.connected` | `func(endpoint str) bool` | `false` |

Anatomy is a direct clone of `ResponseFunc`
(`lang/core/net/http/response.go`): full custom struct implementing
`interfaces.StreamableFunc`, `Info(){Pure: false, ...}`, `ArgGen`, `Copy`,
`funcs.ErrCantSpeculate` guard in `Call` when `obj.init == nil`.
Differences:

- `Stream()` selects on **three** sources: the args-input chan (endpoint+id,
  fixed after first receipt, like response.go's uid); the
  `obj.init.Local.BridgeWatch(ctx, "esphome:endpoint", endpoint)` chan; and
  the session watch chan. On bridge event -> `BridgeGet` ->
  `session.Configure(info)` (nil unpublish included) -> `init.Event`. On
  session event -> `init.Event`.
- `Call()` reads `BridgeGet`; nil -> return the zero value (bare zeros --
  same convention as http.response's "status is zero until something
  happens"). Else `session.State(id)`; nil/missing/wrong-domain -> zero
  value; else the typed value.
- Sessions are refcounted: `SessionReserve` in Stream after the endpoint arg
  arrives, `Release` on Stream exit (deferred).
- Wrong-domain reads (e.g. `binary_sensor()` on a switch id) return zero,
  log under Debug. (Deliberately forgiving for the POC.)

GPIO reads: an ESPHome GPIO input pin is a `binary_sensor` entity in the
device YAML, so `net.esphome.binary_sensor("garage", "button_a")` *is* the
"read GPIO with events" function. Raw pin numbers never cross the API.

**Registration hook:** add
`_ "github.com/purpleidea/mgmt/lang/core/net/esphome"` to
`lang/core/core.go` (alphabetical) or the functions won't register.

## Example mcl (`examples/lang/esphome0.mcl`)

```mcl
import "net/esphome"

esphome:endpoint "garage" {
	host => "192.168.1.50",
	key => "base64-noise-psk-here",
	interval => 0, # native push events
}

$pressed = esphome.binary_sensor("garage", "button_a")

esphome:switch "led_1" {
	endpoint => "garage",
	state => $pressed ? "on" : "off",
}
```

## File map (implementation order)

1. `docs/esphome-plan.md` -- this design document.
2. `go.mod`/`go.sum` -- add `github.com/richard87/esphome-apiclient`
   (resolve the go-1.26.1 directive: toolchain bump or trivial fork).
3. `util/esphome/{esphome,session,driver,apiclient}.go` + fake-driver tests.
4. `engine/resources/esphome.go` -- endpoint resource.
5. `lang/core/net/esphome/{esphome,binary_sensor,sensor,text_sensor,connected}.go`
   + blank import in `lang/core/core.go`.
6. `engine/resources/esphome_entities.go` -- `esphome:switch`,
   `esphome:number`, autoedges.
7. `examples/lang/esphome0.mcl`.

## Verification

- `make build`, `make gofmt`, `./test/test-govet.sh`.
- `go test github.com/purpleidea/mgmt/util/esphome` -- fake-driver unit
  tests: pooling refcounts, Configure idempotency/change, zero-value before
  publish, poll vs push mode, command wake in poll mode.
- Function wiring smoke: `go test -count=1 github.com/purpleidea/mgmt/lang
  -run 'TestAstFunc2/' -short` still passes (no regressions; live-device
  functions can't be exercised there).
- End-to-end against real hardware:
  `./mgmt run --tmp-prefix lang examples/lang/esphome0.mcl` pointed at a
  real ESPHome device (or an `esphome run` firmware in a container/VM with a
  trivial YAML: one `gpio` binary_sensor + one `gpio` switch). Verify: zero
  values before endpoint applies; live updates on button press with
  `interval => 0`; polling cadence with `interval => 5`; out-of-band flips
  (toggle via Home Assistant/web UI) repaired by `esphome:switch`.

## Future work (post-POC)

- `esphome:fan` for H-bridge motor control (direction + speed) -- the
  conveyor demo target shared with go-aioesphomeapi's roadmap.
- `esphome:light`, `esphome:cover`, `esphome:button` (refresh-triggered
  press).
- Optional `{value, ready}` struct variants of the read functions
  (`value.get` pattern) when "zero vs not-yet-connected" must be
  distinguishable in mcl.
- Upstream patches to richard87 (ByObjectID, go directive, ConnectRequest)
  as needed; migrate the driver adapter to `go-aioesphomeapi` at its M1.
- mDNS discovery of devices; a `net.esphome.entities(endpoint)` list
  function.
