# Alarm Button System

[![Go Report Card](https://goreportcard.com/badge/github.com/oshokin/alarm-button)](https://goreportcard.com/report/github.com/oshokin/alarm-button)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A distributed emergency shutdown system designed to quickly and safely power down all office computers when a hungry tiger enters the building elevator and threatens to reach the office floor.

## 🐅 Emergency Scenarios

### Primary Threat: Elevator Tiger Incident

When wildlife experts detect a hungry tiger has entered the building elevator, office safety protocols require immediate evacuation. This system ensures all computers are safely powered down to prevent data loss and equipment damage during the emergency evacuation.

### Additional Tiger Threat Scenarios

#### 🚪 Tiger Stuck in Revolving Door

- Tiger enters revolving door but gets confused by the rotation mechanism
- Keeps going in circles for 10 minutes while growing increasingly frustrated
- Security realizes the situation when they hear rhythmic growling every 30 seconds
- Emergency shutdown prevents tiger from learning how doors work

#### ☕ Tiger in Office Kitchen

- Tiger discovers the coffee machine and becomes highly caffeinated
- Starts frantically pressing alarm buttons while jittery from espresso
- Attempts to use microwave but can't figure out the timer
- System activates when tiger accidentally triggers motion sensors while doing coffee-induced zoomies

#### 📱 Tiger Zoom Meeting Incident

- Tiger accidentally joins all-hands meeting through open laptop
- Entire company sees large cat staring directly into webcam
- Tiger tilts head confused by dozens of tiny human faces on screen  
- Meeting participants can't tell if it's a very realistic virtual background
- System activates when someone finally realizes Steve from Accounting doesn't own a tiger

## 🏗️ Architecture

The system follows a client-server architecture with role-based executable distribution:

### Server Components (Central Control Room)

- **`alarm-server`** - Central gRPC server managing alarm state
- **`alarm-button-off`** - Resets the alarm state (tiger safely captured)
- **`alarm-packager`** - Prepares and distributes software updates

### Client Components (Office Workstations)

- **`alarm-button-on`** - Triggers the emergency alarm (tiger detected!)
- **`alarm-checker`** - Monitors alarm state and initiates shutdown when activated
- **`alarm-updater`** - Keeps client software up-to-date

## 🚀 Quick Start

### Prerequisites

- Go 1.27.1 or later
- [Task](https://taskfile.dev/) build tool
- Network connectivity between server and all client machines

### Platform Support

This system supports the following platforms:

- **Windows** (x64)
- **Linux** (x64)
- **macOS** (x64 and ARM64)

### Installation

#### Option 1: Download Pre-built Binaries (Recommended)

**Simply download the latest release for your platform:**

1. **Go to GitHub Releases**: [https://github.com/oshokin/alarm-button/releases](https://github.com/oshokin/alarm-button/releases)

2. **Download the appropriate archive for your system:**

   | Platform | Architecture | Download Format | Example Filename |
   | ---------- | ------------- | ----------------- | ------------------ |
   | **Windows x64** | amd64 | ZIP | `alarm-button_v1.0.0_windows_amd64.zip` |
   | **Windows ARM64** | arm64 | ZIP | `alarm-button_v1.0.0_windows_arm64.zip` |
   | **Linux x64** | amd64 | tar.gz | `alarm-button_v1.0.0_linux_amd64.tar.gz` |
   | **Linux ARM64** | arm64 | tar.gz | `alarm-button_v1.0.0_linux_arm64.tar.gz` |
   | **macOS x64** | amd64 | tar.gz | `alarm-button_v1.0.0_darwin_amd64.tar.gz` |
   | **macOS ARM64** | arm64 | tar.gz | `alarm-button_v1.0.0_darwin_arm64.tar.gz` |

3. **Extract the archive:**

   ```bash
   # Windows (PowerShell)
   Expand-Archive alarm-button_v1.0.0_windows_amd64.zip -DestinationPath alarm-button
   
   # Linux/macOS
   tar -xzf alarm-button_v1.0.0_linux_amd64.tar.gz
   cd alarm-button
   ```

4. **Verify installation:**

   ```bash
   # Windows
   .\alarm-server.exe version
   
   # Linux/macOS  
   ./alarm-server version
   ```

**Each release contains these ready-to-run executables:**

- `alarm-server` / `alarm-server.exe` - Central gRPC server
- `alarm-button-on` / `alarm-button-on.exe` - Emergency trigger
- `alarm-button-off` / `alarm-button-off.exe` - Reset alarm
- `alarm-checker` / `alarm-checker.exe` - Shutdown monitor daemon
- `alarm-packager` / `alarm-packager.exe` - Update preparation tool
- `alarm-updater` / `alarm-updater.exe` - Auto-updater client
- `README.md` - This documentation

#### Option 2: Build From Source (For Developers)

**Installation:**

```bash
# Clone the repository
git clone https://github.com/oshokin/alarm-button.git
cd alarm-button

# Install development tools
task install-tools

# Build all executables for your platform
task build
```

### Configuration

Create a configuration file `alarm-button-settings.yaml` (or let the system create defaults):

```yaml
server_addr: "control-room-server:8080"
update_folder: "https://updates.company.com/alarm-system/"
state_file: "alarm-button-state.json"
timeout: "30s"
```

## 📦 Deployment

### Server Setup (Control Room)

1. **Download and extract the release for your server platform**
2. **Deploy the alarm server:**

   ```bash
   # Linux/macOS - Uses defaults from alarm-button-settings.yaml
   ./alarm-server
   
   # Windows - Uses defaults from alarm-button-settings.yaml
   .\alarm-server.exe
   
   # OR override listen address
   ./alarm-server :8080           # Linux/macOS
   .\alarm-server.exe :8080       # Windows
   ```

3. **Set up update distribution:**

   ```bash
   # Linux/macOS
   ./alarm-packager \
     --input-dir ./dist/linux_amd64 \
     --output-dir /path/to/update/folder \
     --version 1.5.7 \
     --key-id 2026-03 \
     --private-key /secure/update-signing-key.pem \
     --client-config ./deployment/client/alarm-button-settings.yaml \
     --client-config-revision 42 \
     --server-config ./deployment/server/alarm-button-settings.yaml \
     --server-config-revision 17
   
   # Windows (PowerShell)
   .\alarm-packager.exe `
     --input-dir .\dist\windows_amd64 `
     --output-dir C:\path\to\update\folder `
     --version 1.5.7 `
     --key-id 2026-03 `
     --private-key C:\secure\update-signing-key.pem `
     --client-config .\deployment\client\alarm-button-settings.yaml `
     --client-config-revision 42 `
     --server-config .\deployment\server\alarm-button-settings.yaml `
     --server-config-revision 17
   ```

### Client Setup (Office Workstations)

1. **Download and extract the release for each client platform**
2. **Deploy the alarm checker (runs as service):**

   ```bash
   # Linux/macOS - Uses server address from alarm-button-settings.yaml
   ./alarm-checker
   
   # Windows - Uses server address from alarm-button-settings.yaml
   .\alarm-checker.exe
   
   # OR override server address
   ./alarm-checker control-room-server:8080    # Linux/macOS
   .\alarm-checker.exe control-room-server:8080 # Windows
   ```

3. **Deploy the updater (runs periodically):**

   ```bash
   # Linux/macOS
   ./alarm-updater client
   
   # Windows
   .\alarm-updater.exe client
   ```

4. **Place emergency button on desktop (MINIMAL ARGS FOR DESKTOP SHORTCUTS):**

   ```bash
   # Linux/macOS - ZERO ARGUMENTS (reads config file)
   ./alarm-button-on
   
   # Windows - ZERO ARGUMENTS (reads config file)
   .\alarm-button-on.exe
   ```

## 🎯 Usage

### Real-World Meaning of "Tiger"

"Tiger" is a friendly codename for "something bad just happened, shut things down and leave calmly."
The practical plan behind the joke is simple:

- one brave human (often from accounting) presses `alarm-button-on` immediately;
- `alarm-checker` on workstations starts controlled shutdown while people evacuate;
- this still works without internet, as long as clients can reach `alarm-server` over LAN.

If unexpected guests decide network and electricity should end early, this gives staff a short but useful reaction window.

### 30-Second Runbook (Before The Tiger Learns Wi-Fi)

1. **Trigger:** nearest operator hits `alarm-button-on` (or desktop shortcut).
2. **Verify:** duty engineer confirms alarm propagation via `alarm-server` logs/health.
3. **Evacuate:** humans leave; machines keep shutting down automatically.
4. **Recover:** after all-clear, authorized staff presses `alarm-button-off`.

### Operational Constraints (Even Tigers Have Physics)

- If power disappears instantly, graceful shutdown has the same chance as explaining Excel macros to a tiger.
- UPS is strongly recommended for `alarm-server`, network gear, and critical workstations.
- Internet is optional; LAN connectivity to `alarm-server` is required.

### Emergency Activation

When any tiger scenario is detected in the building:

1. **Any employee can trigger the alarm:**

   ```bash
   # Linux/macOS - ZERO ARGUMENTS (perfect for desktop shortcuts!)
   ./alarm-button-on
   
   # Windows - ZERO ARGUMENTS (perfect for desktop shortcuts!)
   .\alarm-button-on.exe
   ```

   **OR just double-click the desktop shortcut!**

2. **All office computers will automatically shutdown** within seconds as `alarm-checker` services detect the alarm state.

### Reset After Emergency

Once the tiger has been safely captured:

1. **Security personnel reset the alarm:**

   ```bash
   # Linux/macOS - ZERO ARGUMENTS (perfect for desktop shortcuts!)
   ./alarm-button-off
   
   # Windows - ZERO ARGUMENTS (perfect for desktop shortcuts!)
   .\alarm-button-off.exe
   ```

2. **Employees can safely power on their computers** and resume work.

## 🔧 Advanced Configuration

### Alarm Checker Options

```bash
# Minimal usage (uses defaults from alarm-button-settings.yaml)
./alarm-checker                                 # Linux/macOS
.\alarm-checker.exe                             # Windows

# Override server address if needed
./alarm-checker control-room-server:8080        # Linux/macOS
.\alarm-checker.exe control-room-server:8080    # Windows

# Advanced flags (all optional)
./alarm-checker -c custom-settings.yaml         # Custom config file
./alarm-checker -d                              # Debug mode (no shutdown)
```

### Server Options

```bash
# Minimal usage (uses defaults from alarm-button-settings.yaml)
./alarm-server                                  # Linux/macOS
.\alarm-server.exe                              # Windows

# Override listen address if needed
./alarm-server :8080                           # Linux/macOS
.\alarm-server.exe :8080                       # Windows

# Advanced flags (all optional)
./alarm-server -c custom-settings.yaml        # Custom config file
./alarm-server -s custom-state.json           # Custom state file
```

## 🧪 Testing

### Integration Tests

```bash
# Linux/macOS - Start test server (if building from source)
./alarm-server :9999 &

# Windows - Start test server (if building from source)  
start .\alarm-server.exe :9999

# Run integration tests (requires building from source)
go test -count=1 ./internal/integration/...
```

### Simulate Emergency (Debug Mode)

```bash
# Linux/macOS - Start checker in debug mode (won't actually shutdown)
./alarm-checker -d localhost:8080 &

# Windows - Start checker in debug mode
start .\alarm-checker.exe -d localhost:8080

# Trigger alarm - Linux/macOS (ZERO ARGS - reads from config)
./alarm-button-on

# Trigger alarm - Windows (ZERO ARGS - reads from config)
.\alarm-button-on.exe

# Check logs - should show "Would shutdown now (debug mode)"
```

## 🔨 Development

### Semantic Versioning and Commit Messages

This project uses semantic versioning with automated releases. All commit messages must follow a specific format:

- **`fix:`** - Bug fixes (increments patch version: 1.0.0 → 1.0.1)
- **`feat:`** - New features (increments minor version: 1.0.0 → 1.1.0)  
- **`major:`** - Breaking changes (increments major version: 1.0.0 → 2.0.0)

#### Examples

```bash
git commit -m "fix: resolve shutdown timeout on Windows"
git commit -m "feat: add authentication to gRPC server"
git commit -m "major: redesign API with breaking changes"
```

#### Setting Up Git Hooks

```bash
# Enable commit message validation (recommended)
task install-githooks

# Disable if needed
task remove-githooks
```

Once hooks are enabled, invalid commit messages will be rejected locally.

### Version Injection

The build system automatically detects semantic version tags and injects version information into binaries:

- **With tags**: `task build` creates versioned binaries (e.g., v1.4.2)
- **Without tags**: `task build` creates development binaries (no version injection)

To create a version tag:

```bash
git tag v1.0.0    # Creates semantic version tag
task build        # Now builds with version 1.0.0 injected
```

See the Build Tasks section for all available development commands.

## ⚙️ Build Tasks

This project uses [Task](https://taskfile.dev/) as a build tool. Below are all available tasks:

### Core Development Tasks

```bash
task build                    # Build all project binaries
task clean                    # Remove all built binaries
task test                     # Run all tests with verbose output
task test-race                # Run tests with race detector enabled
task generate-protobuf        # Generate protobuf code and format
task version-check            # Check next semantic version from commit history
```

### Linting Tasks

```bash
task lint                     # Run golangci-lint
task lint-fix                 # Run golangci-lint with auto-fix
task security                 # Run govulncheck scan
```

### Installation Tasks

```bash
task install-tools            # Install all development tools
task install-lint             # Install golangci-lint
task install-protoc           # Install protoc compiler
task install-protoc-gen-go    # Install protoc-gen-go plugin
task install-protoc-gen-go-grpc # Install protoc-gen-go-grpc plugin
task install-githooks         # Configure Git hooks for semantic commit enforcement
task remove-githooks          # Disable Git hooks for this repository
```

### Version Management Tasks

```bash
task fetch-tags               # Fetch latest tags from remote
task version-check            # Check next semantic version from commit history
```

### Task Usage Examples

```bash
# First-time setup
task install-tools

# Development workflow
task generate-protobuf # Generate protobuf if API changed
task lint-fix        # Fix linting issues
task test            # Run tests
task build           # Build binaries

# Quality checks
task lint            # Check lint rules
task test-race       # Run with race detection
task security        # Run vulnerability checks
```

## 📁 Project Structure

```bash
alarm-button/
├── cmd/                    # Application entry points
│   ├── alarm-server/       # Central alarm state server
│   ├── alarm-button-on/    # Emergency trigger client
│   ├── alarm-button-off/   # Reset client
│   ├── alarm-checker/      # Shutdown monitor daemon
│   ├── alarm-packager/     # Update preparation tool
│   └── alarm-updater/      # Auto-updater client
├── internal/               # Private application code
│   ├── api/grpc/alarm/     # gRPC service implementation
│   ├── config/             # Configuration management
│   ├── domain/alarm/       # Core business logic
│   ├── logger/             # Structured logging
│   ├── repository/state/   # State persistence
│   └── service/            # Business services
├── api/v1/                 # Protocol buffer definitions
└── scripts/                # Build and deployment scripts
```

## 🖥️ Platform-Specific Notes

### Windows

- Executables have `.exe` extension
- Use `^` for line continuation in Command Prompt or `` ` `` in PowerShell
- Default paths: `C:\ProgramData\alarm\` for system-wide files
- Consider running as Windows Service for `alarm-checker`

### Linux

- No file extensions for executables
- Use `\` for line continuation in bash
- Default paths: `/var/lib/alarm/` for system-wide files
- Use systemd for service management
- Requires appropriate permissions for shutdown commands

### macOS

- Same as Linux for most operations
- Support for both Intel (x64) and Apple Silicon (ARM64)
- Use launchd for service management
- May require additional permissions for shutdown operations

## 🔐 Security Considerations

- **Network Security**: Use TLS for production deployments
- **Access Control**: Restrict who can trigger alarms via system permissions
- **State Persistence**: Alarm state survives server restarts
- **Graceful Shutdown**: Computers shutdown safely to prevent data loss
- **Update Security**: Verify checksums for all distributed updates
- **Cross-Platform**: Same security model applies to all supported platforms

### Update Signing Key Ceremony

Treat signing keys like tiger fangs: shiny in public, dangerous in private.

1. Generate a new signing key offline (`keygen` creates the Ed25519 pair itself and prints trusted public key + recommended key ID):

   Linux/macOS:

   ```bash
   go run ./cmd/alarm-packager keygen --private-key /secure/update-signing-key.pem --key-id 2026-03
   ```

   Windows (PowerShell):

   ```powershell
   go run .\cmd\alarm-packager keygen --private-key C:\secure\update-signing-key.pem --key-id 2026-03
   ```

   If you omit `--key-id`, `keygen` auto-suggests `YYYY-MM` in UTC.

2. Keep the private key out of git forever (vault/HSM/CI secret store only).
3. Publish only the public key in `internal/service/updater/signature.go`.
4. Sign release artifacts with `alarm-packager` using that private key.
5. Run a smoke test: `alarm-updater` must accept the signature and apply update from HTTP repository.

## 🚀 Automated Releases

This project uses automated semantic versioning and releases:

### How It Works

1. **Commit with semantic prefix** (see [Semantic Versioning and Commit Messages](#semantic-versioning-and-commit-messages))
2. **Push to master**: GitHub Actions analyzes commits since last release
3. **Automatic version bump**: Script determines version increment
4. **Tag creation**: New semantic version tag is created (e.g., `v1.4.8`)
5. **Release build**: GoReleaser creates cross-platform binaries
6. **GitHub Release**: Assets are published automatically

### Release Types

Release type mapping is defined once in the Development section (`fix:` / `feat:` / `major:`).

### Manual Release (if needed)

```bash
# Create and push a tag manually
git tag v1.0.0
git push origin v1.0.0
```

## 🛠️ IDE Support

### VSCode Debugging

The project includes comprehensive debugging configurations in `.vscode-example/launch.json`:

- **alarm-server** debugging (default and custom ports)
- **alarm-button-on/off** debugging (with safety debug flags)
- **alarm-checker** debugging (safe mode, no actual shutdown)
- **alarm-updater** debugging (client and server roles)
- **Integration tests** debugging
- **All-in-one scenarios** for complex testing

To use: Open VSCode, go to Run & Debug (Ctrl+Shift+D), and select any configuration.

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Use semantic commits (`git commit -m 'feat: add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Code Style

- Follow standard Go conventions
- Add package documentation for all packages (`doc.go` files)
- Include unit tests for new functionality
- Use semantic commit messages (`fix:`, `feat:`, `major:`)
- Run `task lint` before submitting
- All comments must start with a capital letter and end with a period

### Development Workflow

Use the canonical command sets from these sections in this README:

- `Build Tasks`
- `Examples`
- `Semantic Versioning and Commit Messages`

## 📋 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## ⚠️ Legal Disclaimer

This software is designed for emergency situations involving hungry tigers entering office buildings via elevators and related feline workplace incidents.

### Practical Limits and Scope

- This software orchestrates technical shutdown; it does not replace physical safety procedures.
- For non-tiger emergencies (fire, medical, security), follow official emergency response first.
- "Tiger" includes any urgent on-site incident where fast workstation shutdown reduces risk.
- If the tiger (or any equivalent chaos source) outruns your power backup plan, people still come first, laptops second.

---

## 🐅 **Final Notice**

**Remember**: In case of an actual hungry tiger emergency, prioritize personal safety over data backup procedures. The software will handle the technical aspects automatically.

*If a tiger is reading this documentation, please note that we have many delicious snacks in the break room and the computers taste terrible.*
