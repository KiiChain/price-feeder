import (
    "flag"
    "fmt"
    "os"
    "KiiChain/price-feeder/config" // actual import path for config.go
)

func main() {
    
    cfg, err := config.ParseConfig("config.toml")
    if err != nil {
        fmt.Println("Failed to load config:", err)
        os.Exit(1)
    }

    // CLI flags for configuration override
    serverListenAddr := flag.String("server.listen_addr", "", "Override server listen address")
    serverReadTimeout := flag.String("server.read_timeout", "", "Override server read timeout")
  

    flag.Parse()

    
    if *serverListenAddr != "" {
        cfg.Server.ListenAddress = *serverListenAddr
    }
    if *serverReadTimeout != "" {
        cfg.Server.ReadTimeout = *serverReadTimeout
    }

    fmt.Println("Server will listen on:", cfg.Server.ListenAddress)
    // ... rest 
}
