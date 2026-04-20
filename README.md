# gameperf

Real-time game performance diagnostics and auto-fix tool for Linux.

`gameperf` monitors system metrics while a game is running, identifies the root cause of performance issues (IRQ misrouting, VRAM pressure, CPU bottlenecks), and produces detailed reports with actionable fix instructions — including automatic remediation where possible.

Built with FF7 Rebirth on Proton/Steam in mind, but applicable to any Linux game.

- [gameperf](#gameperf)
  - [Features](#features)
  - [Requirements](#requirements)
  - [Installation](#installation)
    - [Build from source](#build-from-source)
    - [Run without installing](#run-without-installing)
  - [Usage](#usage)
    - [`diagnose` — one-shot analysis](#diagnose--one-shot-analysis)
    - [`fix` — diagnose and apply all auto-fixable issues](#fix--diagnose-and-apply-all-auto-fixable-issues)
    - [`monitor` — continuous re-diagnosis](#monitor--continuous-re-diagnosis)
    - [`report` — write a full report to a file](#report--write-a-full-report-to-a-file)
    - [`serve` — Prometheus metrics endpoint](#serve--prometheus-metrics-endpoint)
    - [Common flags](#common-flags)
  - [Prometheus + Grafana monitoring stack](#prometheus--grafana-monitoring-stack)
    - [Metrics exposed](#metrics-exposed)
    - [Quickstart](#quickstart)
    - [Existing Prometheus setup](#existing-prometheus-setup)
  - [LLM-powered analysis](#llm-powered-analysis)
    - [Mode 1 — Ollama (local, free, recommended)](#mode-1--ollama-local-free-recommended)
    - [Mode 2 — OpenAI (cloud, requires API key)](#mode-2--openai-cloud-requires-api-key)
    - [Mode 3 — Google Gemini (cloud, requires API key)](#mode-3--google-gemini-cloud-requires-api-key)
    - [Mode 4 — Anthropic Claude (cloud, requires API key)](#mode-4--anthropic-claude-cloud-requires-api-key)
    - [Config file reference](#config-file-reference)
    - [CLI flag overrides](#cli-flag-overrides)
    - [What the LLM does and does not do](#what-the-llm-does-and-does-not-do)
  - [Diagnostic rules](#diagnostic-rules)
    - [`irq-ecore-*` — GPU IRQ routed to E-core 🔴 Critical](#irq-ecore---gpu-irq-routed-to-e-core--critical)
    - [`vram-pressure` — VRAM near full 🟡 Warning / 🔴 Critical](#vram-pressure--vram-near-full--warning---critical)
    - [`game-not-running` — no game process found 🔵 Info](#game-not-running--no-game-process-found--info)
    - [`cpu-governor-powersave` — P-cores using powersave governor 🔴 Critical](#cpu-governor-powersave--p-cores-using-powersave-governor--critical)
    - [`vm-max-map-count` — Max memory map count too low 🟡 Warning](#vm-max-map-count--max-memory-map-count-too-low--warning)
    - [`thp-always` — Transparent Huge Pages set to 'always' 🟡 Warning](#thp-always--transparent-huge-pages-set-to-always--warning)
    - [`swap-pressure` — High swap usage 🟡 Warning / 🔴 Critical](#swap-pressure--high-swap-usage--warning---critical)
    - [`ram-pressure` — Low available RAM 🟡 Warning](#ram-pressure--low-available-ram--warning)
    - [`cpu-throttling` — P-cores throttled below max frequency 🟡 Warning](#cpu-throttling--p-cores-throttled-below-max-frequency--warning)
    - [`irqbalance-missing` — irqbalance not running 🟡 Warning](#irqbalance-missing--irqbalance-not-running--warning)
  - [Architecture](#architecture)
  - [Development](#development)
  - [Known limitations](#known-limitations)


---

## Features

- **IRQ E-core detection** — detects GPU interrupt lines (NVIDIA, i915, amdgpu) routed to slow efficiency cores and pins them to performance cores automatically
- **VRAM pressure analysis** — warns when GPU memory is near full, which causes texture eviction stalls and multi-frame hitches
- **CPU governor detection** — detects P-cores throttled by the powersave governor and auto-fixes to performance
- **Memory pressure analysis** — warns on low available RAM and high swap usage before they cause stutter
- **Transparent Huge Pages check** — detects THP=always and auto-fixes to madvise to eliminate compaction stalls
- **vm.max_map_count check** — detects the default 65530 limit that crashes many Vulkan/DX12 games
- **CPU throttling detection** — identifies P-cores running well below rated frequency (thermal/power limits)
- **irqbalance monitoring** — warns when irqbalance is not running and all IRQs pile up on CPU0
- **Game process detection** — confirms the game is running before drawing conclusions
- **Three output formats** — human-friendly console, Markdown report, and JSON for scripting
- **Auto-fix support** — safe fixes are applied in one command (`gameperf fix --sudo`)
- **Continuous monitoring** — re-diagnoses on a configurable interval with `monitor`
- **Prometheus + Grafana integration** — expose all metrics in real-time via `gameperf serve`, visualise with the bundled Grafana dashboard using a single `docker compose up`
- **LLM-powered analysis** — send the diagnostic report to an LLM (Ollama locally or OpenAI) for expert root-cause analysis and prioritised fix recommendations

---

## Prerequisites

### Required

| Dependency | Why | Install |
|---|---|---|
| **Linux x86-64** | Only supported OS | — |
| **Go 1.24+** | Build from source | [go.dev/dl](https://go.dev/dl/) |

### Runtime tools (auto-fixes won't work without these)

| Tool | Used for | Install (Debian/Ubuntu) | Install (Arch/Fedora) |
|---|---|---|---|
| `sudo` | Applying privileged fixes | pre-installed | pre-installed |
| `sysctl` | Fixing `vm.max_map_count` | `procps` | `procps-ng` |
| `systemctl` | Enabling `irqbalance` service | `systemd` | `systemd` |
| `irqbalance` | IRQ affinity balancing | `apt install irqbalance` | `pacman -S irqbalance` / `dnf install irqbalance` |
| `cpupower` | Persisting CPU governor changes (optional) | `apt install linux-tools-common` | `pacman -S cpupower` / `dnf install kernel-tools` |
| `lm-sensors` / `sensors` | CPU temperature readings (manual fix hints) | `apt install lm-sensors` | `pacman -S lm_sensors` / `dnf install lm_sensors` |

> **Note:** gameperf reads `/proc` and `/sys` directly for most diagnostics — the tools above are only needed when *applying* fixes.

### GPU metrics (optional)

| GPU | Requirement |
|---|---|
| NVIDIA | `nvidia-smi` on `$PATH` — included in the [NVIDIA driver package](https://www.nvidia.com/drivers) |
| AMD / Intel | Not yet supported — metrics will show `0` |

### Monitoring stack (optional)

| Tool | Why | Install |
|---|---|---|
| **Docker** | Run Prometheus + Grafana | [docs.docker.com/get-docker](https://docs.docker.com/get-docker/) |
| **Docker Compose** | One-command stack spin-up | Bundled with Docker Desktop; `apt install docker-compose-plugin` on Linux |

### LLM analysis (optional)

| Provider | Requirement |
|---|---|
| Ollama (local) | [ollama.com](https://ollama.com) installed and running |
| OpenAI | API key from [platform.openai.com](https://platform.openai.com/api-keys) |
| Google Gemini | API key from [aistudio.google.com](https://aistudio.google.com/app/apikey) |
| Anthropic Claude | API key from [console.anthropic.com](https://console.anthropic.com/) |

---

## Installation

### Build from source

```bash
git clone https://github.com/zenardi/gameperf.git
cd gameperf
make build          # produces dist/gameperf
make install        # installs to $(go env GOPATH)/bin
```

After `make install`, ensure Go's bin directory is in your `$PATH`.
Add this line to your shell config (`~/.zshrc`, `~/.bashrc`, etc.) if it isn't already:

```bash
export PATH=$PATH:$(go env GOPATH)/bin
```

Then reload your shell:

```bash
source ~/.zshrc   # or source ~/.bashrc
```

You can verify the install worked with:

```bash
gameperf --help
```

### Run without installing

```bash
make run
```

---

## Usage

### `diagnose` — one-shot analysis

```bash
gameperf diagnose
gameperf diagnose --fix --sudo      # diagnose and auto-apply fixes
gameperf diagnose --format json     # machine-readable output
```

### `fix` — diagnose and apply all auto-fixable issues

```bash
gameperf fix
gameperf fix --sudo     # prepend sudo to commands that require root
```

### `monitor` — continuous re-diagnosis

```bash
gameperf monitor                    # refresh every 10 seconds (default)
gameperf monitor --interval 5       # refresh every 5 seconds
```

### `report` — write a full report to a file

```bash
gameperf report                             # writes gameperf-report.md
gameperf report --output /tmp/report.json --format json
```

### `analyze-file` — send a saved report to the LLM

After a gaming session you may already have a report file on disk. Use this
command to send it to the LLM **without needing to collect new metrics** — the
game and gameperf do not need to be running.

```bash
gameperf analyze-file gameperf-report.md
gameperf analyze-file /tmp/session.json --llm-provider gemini --llm-model gemini-2.5-flash
```

The file contents (markdown, JSON, or plain text) are sent verbatim to the
LLM. The AI analysis is printed to stdout.

### `serve` — Prometheus metrics endpoint

```bash
gameperf serve                              # listen on :9100, collect every 5s
gameperf serve --port 9200 --interval 2    # custom port and interval
```

Exposes all real-time metrics at `http://localhost:9100/metrics` in Prometheus
exposition format. Point your Prometheus instance at it, or spin up the full
Prometheus + Grafana monitoring stack with one command (see below).

### Common flags

| Flag | Default | Description |
|---|---|---|
| `--game` | *(auto-detect)* | Override auto-detected game (process name substrings) |
| `--format` | `console` (`markdown` for `report`) | Output format: `console`, `markdown`, `json` |
| `--fix` | `false` | Auto-apply fixes after diagnosing (`diagnose` only) |
| `--sudo` | `false` | Prepend `sudo` to fix commands that require root |
| `--interval` | `10` | Seconds between runs (`monitor` only) |
| `--output` | `gameperf-report.md` | Output file path (`report` only) |
| `--port` | `9100` | Port for the `/metrics` HTTP server (`serve` only) |
| `--interval` | `5` | Seconds between collections (`serve` only) |
| `--llm` | `false` | Enhance output with AI analysis (`diagnose`, `report`) |
| `--llm-provider` | *(from config)* | Override LLM provider: `ollama`, `openai`, `gemini`, `anthropic` |
| `--llm-model` | *(from config)* | Override LLM model name |

### Game auto-detection

gameperf automatically detects running games without any configuration by inspecting `/proc` for:

| Signal | What it covers |
|---|---|
| `SteamAppId` env var | Any game launched by Steam (native Linux or Proton) |
| exe path under `steamapps/common/` | Native Steam games not setting the env var |
| `LUTRIS_GAME_UUID` env var | Games launched via Lutris |

Steam infrastructure processes (`steamwebhelper`, `reaper`, `pressure-vessel`, etc.) are automatically excluded.

If no game is auto-detected, `--game` can be used as a manual override:

```bash
gameperf diagnose --game cyberpunk2077
gameperf diagnose --game elden,ring   # comma-separated substrings
```

---

## Prometheus + Grafana monitoring stack

`gameperf serve` exposes all metrics in real-time. The repo ships a complete
Docker Compose stack that wires Prometheus and Grafana together automatically —
no manual configuration needed.

### Metrics exposed

| Metric | Type | Description |
|---|---|---|
| `gameperf_cpu_usage_percent{core}` | Gauge | Per-core CPU usage (delta between collections) |
| `gameperf_cpu_governor_info{core,governor}` | Gauge | Active scaling governor per core (value=1) |
| `gameperf_cpu_throttle_percent{core}` | Gauge | How far below rated frequency each P-core is running |
| `gameperf_gpu_vram_used_mib` | Gauge | GPU VRAM used in MiB |
| `gameperf_gpu_vram_total_mib` | Gauge | GPU VRAM total capacity in MiB |
| `gameperf_gpu_vram_used_percent` | Gauge | GPU VRAM used as % of total |
| `gameperf_ram_available_percent` | Gauge | RAM available as % of total |
| `gameperf_swap_used_percent` | Gauge | Swap used as % of total swap |
| `gameperf_vm_max_map_count` | Gauge | Current value of `vm.max_map_count` |
| `gameperf_finding_active{id,severity}` | Gauge | 1 when a diagnostic rule is currently firing |

### Quickstart

**Step 1 — start the metrics server** (on your gaming machine):

```bash
gameperf serve                          # exposes http://localhost:9100/metrics
```

**Step 2 — configure your host IP** (required for rootless Docker):

```bash
# Find your LAN/Wi-Fi IP (not 127.0.0.1)
hostname -I | awk '{print $1}'

# Create the .env file (already present if you cloned the repo)
echo "HOST_IP=192.168.0.x" > .env      # replace with your actual IP
```

> **Why?** Prometheus runs inside Docker and cannot reach `localhost` on the
> host when using rootless Docker. The `.env` file tells Docker Compose which
> IP to use. The `.env` file is git-ignored so your IP is never committed.

**Step 3 — start Prometheus + Grafana**:

```bash
docker compose up -d
```

| Service | URL | Credentials |
|---|---|---|
| Grafana | http://localhost:3000 | admin / gameperf |
| Prometheus | http://localhost:9090 | — |

The bundled dashboard (`grafana/dashboard.json`) is provisioned automatically.
It includes panels for CPU usage, CPU governors, CPU throttle, GPU VRAM,
RAM/swap pressure, vm.max_map_count, and a live active-findings table.

### Existing Prometheus setup

If you already run Prometheus, add this scrape job to your config:

```yaml
scrape_configs:
  - job_name: gameperf
    static_configs:
      - targets: ["<your-machine-ip>:9100"]
    scrape_interval: 5s
```

Then import `grafana/dashboard.json` into your existing Grafana instance via
**Dashboards → Import → Upload JSON file**.

---

## LLM-powered analysis

`gameperf` can send the diagnostic report to an LLM and receive expert
root-cause analysis with prioritised recommendations.

### Mode 1 — Ollama (local, free, recommended)

No API key, no internet. Everything runs on your machine.

```bash
# Install Ollama
curl -fsSL https://ollama.com/install.sh | sh
ollama pull llama3.2

# Run gameperf with AI analysis
gameperf diagnose --llm
gameperf report --llm          # AI section appended to the markdown file
```

### Mode 2 — OpenAI (cloud, requires API key)

```bash
mkdir -p ~/.config/gameperf
cat > ~/.config/gameperf/config.toml << 'EOF'
[llm]
provider = "openai"
model    = "gpt-4o-mini"
api_key  = "sk-..."
EOF

gameperf diagnose --llm
```

### Mode 3 — Google Gemini (cloud, requires API key)

Get a free API key at [aistudio.google.com](https://aistudio.google.com/app/apikey).

```bash
mkdir -p ~/.config/gameperf
cat > ~/.config/gameperf/config.toml << 'EOF'
[llm]
provider = "gemini"
model    = "gemini-2.5-flash"
api_key  = "AIza..."
EOF

gameperf diagnose --llm
```

### Mode 4 — Anthropic Claude (cloud, requires API key)

Get an API key at [console.anthropic.com](https://console.anthropic.com/).

```bash
mkdir -p ~/.config/gameperf
cat > ~/.config/gameperf/config.toml << 'EOF'
[llm]
provider = "anthropic"
model    = "claude-3-5-haiku-20241022"
api_key  = "sk-ant-..."
EOF

gameperf diagnose --llm
```

### Config file reference

`~/.config/gameperf/config.toml` — created manually, never auto-generated.

```toml
[llm]
provider = "ollama"           # "ollama" (default), "openai", "gemini", or "anthropic"
model    = "llama3.2"         # any model available in your provider
api_key  = ""                 # required for openai/gemini/anthropic; leave empty for ollama
url      = ""                 # override base URL; empty = provider default
```

### CLI flag overrides

Flags take precedence over the config file for one-off overrides:

```bash
gameperf diagnose --llm --llm-provider openai --llm-model gpt-4o
```

### What the LLM does and does not do

| ✅ Does | ❌ Does not |
|---|---|
| Analyse finding interactions and root causes | Execute any shell commands |
| Prioritise fixes by impact | Auto-apply fixes (that's the deterministic fixer) |
| Explain why a metric is concerning | Access the internet during analysis |
| Suggest in-game or system changes | Store or transmit your API key |

---

## Diagnostic rules

### `irq-ecore-*` — GPU IRQ routed to E-core 🔴 Critical

On hybrid CPUs (Intel Core Ultra, 12th gen+), the kernel or `irqbalance` may route GPU interrupts to slow efficiency cores. Every frame delivery triggers an interrupt; if that interrupt is handled on an E-core, the kernel spends time there instead of immediately waking the render thread on a P-core. This produces consistent micro-stutters.

**Auto-fix:** pins the IRQ's `smp_affinity_list` to P-cores (`0–N`).

```bash
echo 0-7 | sudo tee /proc/irq/217/smp_affinity_list
```

### `vram-pressure` — VRAM near full 🟡 Warning / 🔴 Critical

When VRAM exceeds ~85%, the GPU must evict texture pages to system RAM on scene transitions, causing multi-frame stalls. Frame Generation (DLSS-FG / FSR-FG) adds ~1.5 GB on top — critical on 8 GB GPUs.

**Not auto-fixable.** Recommended in-game actions:
- Lower texture quality by one step
- Disable Frame Generation in OptiScaler settings

### `game-not-running` — no game process found 🔵 Info

No matching process was found. Metrics reflect idle system state and may not represent in-game conditions. Launch the game and re-run.

### `cpu-governor-powersave` — P-cores using powersave governor 🔴 Critical

The `powersave` CPU governor caps frequency far below the rated maximum. Any CPU burst the game needs (physics, AI, streaming) is immediately throttled, causing frame stutter. On Intel hybrid CPUs, this affects P-cores most.

**Auto-fix:** sets all CPUs to `performance`.

```bash
echo performance | sudo tee /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor
```

### `vm-max-map-count` — Max memory map count too low 🟡 Warning

The default Linux kernel limit of 65530 memory-mapped regions is too low for many games (Elden Ring, DX12/Vulkan titles, large open worlds). Hitting the limit causes crashes or multi-second freezes.

**Auto-fix:** raises it to 1048576 via sysctl.

### `thp-always` — Transparent Huge Pages set to 'always' 🟡 Warning

`THP=always` causes the kernel to periodically compact memory pages into 2 MB blocks, creating unpredictable stall spikes. `madvise` is better — Wine/Proton already opts into THP where it helps.

**Auto-fix:** switches to `madvise`.

### `swap-pressure` — High swap usage 🟡 Warning / 🔴 Critical

Active swap means the OS is paging game assets to disk. Texture loads that normally take microseconds take milliseconds from an NVMe (or seconds from HDD), causing hitching.

**Not auto-fixable.** Close background applications before launching the game.

### `ram-pressure` — Low available RAM 🟡 Warning

Less than 2 GiB of free RAM available. Modern games require 8–16 GiB; low availability forces early texture eviction to swap.

**Not auto-fixable.** Close memory-heavy applications (browsers, VMs).

### `cpu-throttling` — P-cores throttled below max frequency 🟡 Warning

P-cores running more than 30% below their rated clock. Typically thermal throttling (CPU too hot) or platform power limits. Causes sustained FPS drops not visible as individual spikes.

**Not auto-fixable.** Check temperature with `sensors`, re-paste CPU if > 95 °C, or reduce in-game quality settings.

### `irqbalance-missing` — irqbalance not running 🟡 Warning

Without irqbalance, all hardware interrupts default to CPU0. Under gaming load (GPU, NVMe, network), CPU0 saturates with interrupt handling and cannot serve the render thread, causing stutter.

**Auto-fix:** enables and starts irqbalance via systemd.

---

## Architecture

```
gameperf/
├── cmd/gameperf/         # CLI entry point (cobra)
├── docker-compose.yml    # Prometheus + Grafana stack
├── .env.example          # Template for HOST_IP (copy to .env and fill in)
├── prometheus/
│   └── gameperf.yml      # Prometheus scrape config
├── grafana/
│   ├── dashboard.json    # Pre-built Grafana dashboard
│   └── provisioning/     # Auto-provisioned datasource + dashboard
└── internal/
    ├── collector/        # Low-level metric readers
    │   ├── irq.go        # /proc/interrupts parser
    │   ├── cpu.go        # /proc/stat + /proc/cpuinfo parser
    │   ├── gpu.go        # nvidia-smi CSV parser
    │   ├── memory.go     # /proc/meminfo + THP parser
    │   ├── sysctl.go     # CPU governor + freq + vm.max_map_count
    │   └── process.go    # /proc/<pid>/comm scanner
    ├── analyzer/         # Diagnostic rules engine
    │   ├── finding.go    # Finding, Report, Severity types
    │   └── analyzer.go   # Collect() + Analyze() + rules
    ├── fixer/            # Fix executor
    │   └── fixer.go      # Apply() / ApplyAll()
    ├── metrics/          # Prometheus metrics registry
    │   └── metrics.go    # Metrics struct, UpdateFromSnapshot, Handler
    ├── llm/              # LLM provider abstraction
    │   ├── llm.go        # Provider interface
    │   ├── config.go     # LLMConfig, LoadConfig, NewFromConfig
    │   ├── enhance.go    # BuildPrompt, EnhanceReport
    │   ├── ollama.go     # Ollama HTTP client
    │   ├── openai.go     # OpenAI chat completions client
    │   ├── gemini.go     # Google Gemini generateContent client
    │   └── anthropic.go  # Anthropic Messages API client
    └── report/           # Output formatters
        └── report.go     # WriteConsole / WriteMarkdown / WriteJSON
```

All collector parsers accept `io.Reader` (or a plain string for GPU output), making them fully testable without touching the real filesystem.

---

## Development

```bash
make test       # run all unit tests
make build      # compile to dist/gameperf
make clean      # remove dist/
```

Tests cover all collector parsers, analyzer rules, and report formatters with synthetic inputs — no real `/proc` filesystem or GPU required.

---

## Known limitations

- GPU metrics require NVIDIA (`nvidia-smi`). AMD/Intel GPU support is planned.
- IRQ affinity changes are not persistent across reboots. See the `ManualFix` instructions in each finding for a permanent solution via `udev` or `/etc/rc.local`.
- `gamescope` integration (to bypass the compositor on Wayland) is not yet implemented.
