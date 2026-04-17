# tmhi-gateway

[![Go Reference](https://pkg.go.dev/badge/github.com/hugoh/tmhi-gateway.svg)](https://pkg.go.dev/github.com/hugoh/tmhi-gateway)
[![codecov](https://codecov.io/gh/hugoh/tmhi-gateway/graph/badge.svg?token=MCZUXN8MHO)](https://codecov.io/gh/hugoh/tmhi-gateway)
[![CI](https://github.com/hugoh/tmhi-gateway/actions/workflows/ci.yml/badge.svg)](https://github.com/hugoh/tmhi-gateway/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/hugoh/tmhi-gateway)](https://goreportcard.com/report/github.com/hugoh/tmhi-gateway)
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fhugoh%2Ftmhi-gateway.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2Fhugoh%2Ftmhi-gateway?ref=badge_shield)

Go library for interacting with T-Mobile Home Internet gateways (Nokia and Arcadyan models).

## Features

- Authenticate with Nokia and Arcadyan gateways
- Reboot gateway devices
- Retrieve signal strength information
- Check gateway status
- Make custom HTTP requests to gateway APIs

## Installation

```bash
go get github.com/hugoh/tmhi-gateway
```

## Quick Start

```go
package main

import (
    "fmt"
    "time"

    gateway "github.com/hugoh/tmhi-gateway"
)

func main() {
    cfg := &gateway.GatewayConfig{
        IP:       "192.168.12.1",
        Username: "admin",
        Password: "your-password",
        Timeout:  5 * time.Second,
    }

    gw := gateway.NewArcadyanGateway(cfg)

    result, err := gw.Login()
    if err != nil {
        panic(err)
    }

    fmt.Println("Logged in:", result.Success)
}
```

## Supported Gateways

- **Nokia** (`NewNokiaGateway(cfg)`)
- **Arcadyan** (`NewArcadyanGateway(cfg)`)

## API

### Gateway Interface

All gateway implementations satisfy the `Gateway` interface:

```go
type Gateway interface {
    Login() (*LoginResult, error)
    Reboot() error
    Request(method, path string) (*InfoResult, error)
    Info() (*InfoResult, error)
    Status() (*StatusResult, error)
    Signal() (*SignalResult, error)
}
```

### Response Types

- `LoginResult` - Authentication result with token/session info
- `StatusResult` - Gateway status check result
- `SignalResult` - Signal strength information (4G/5G metrics)
- `InfoResult` - Gateway information response

## Development

### Prerequisites

- [mise](https://mise.jdx.dev/) (task runner and tool manager)

### Running Tests

```bash
# Run all tests
mise test

# Run CI checks (lint + test + coverage)
mise ci
```

## License

MIT License - see [LICENSE](LICENSE) for details.


[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fhugoh%2Ftmhi-gateway.svg?type=large)](https://app.fossa.com/projects/git%2Bgithub.com%2Fhugoh%2Ftmhi-gateway?ref=badge_large)