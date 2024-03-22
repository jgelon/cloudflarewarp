// Package cloudflarewarp Traefik Plugin.
package cloudflarewarp

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/BetterCorp/cloudflarewarp/ips"
)

const (
	xRealIP        = "X-Real-Ip"
	xCfTrusted     = "X-Is-Trusted"
	xForwardFor    = "X-Forwarded-For"
	xForwardProto  = "X-Forwarded-Proto"
	cfConnectingIP = "CF-Connecting-IP"
	cfVisitor      = "CF-Visitor"
	tickerInterval = 1 * time.Minute
)

// Config the plugin configuration.
type Config struct {
	TrustIP             []string `json:"trustip,omitempty"`
	DisableDefaultCFIPs bool     `json:"disableDefault,omitempty"`
	TrustDNSName        string   `json:"trustDnsName,omitempty"`
	ClusterCIDR         []string `json:"clusterCIDR,omitempty"`
}

// TrustResult for Trust IP test result.
type TrustResult struct {
	isFatal  bool
	isError  bool
	trusted  bool
	directIP string
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		TrustIP:             []string{},
		DisableDefaultCFIPs: false,
		TrustDNSName:        "",
		ClusterCIDR:         []string{},
	}
}

// RealIPOverWriter is a plugin that overwrite true IP.
type RealIPOverWriter struct {
	next       http.Handler
	name       string
	TrustIP    []*net.IPNet
	ticker     *time.Ticker
	tickerQuit chan struct{}
	dnsName    string
	clusterNet []*net.IPNet
}

// CFVisitorHeader definition for the header value.
type CFVisitorHeader struct {
	Scheme string `json:"scheme"`
}

// New created a new plugin.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	ipOverWriter := &RealIPOverWriter{
		next: next,
		name: name,
	}

	if config.TrustIP != nil {
		for _, v := range config.TrustIP {
			_, trustip, err := net.ParseCIDR(v)
			if err != nil {
				return nil, err
			}

			ipOverWriter.TrustIP = append(ipOverWriter.TrustIP, trustip)
		}
	}

	if !config.DisableDefaultCFIPs {
		for _, v := range ips.CFIPs() {
			_, trustip, err := net.ParseCIDR(v)
			if err != nil {
				return nil, err
			}

			ipOverWriter.TrustIP = append(ipOverWriter.TrustIP, trustip)
		}
	}

	if len(config.TrustDNSName) > 0 {
		ipOverWriter.dnsName = config.TrustDNSName
		ipOverWriter.UpdateTrusted(false)
		ipOverWriter.ticker = time.NewTicker(tickerInterval)
		ipOverWriter.tickerQuit = make(chan struct{})
		go func() {
			for {
				select {
				case <-ipOverWriter.ticker.C:
					ipOverWriter.UpdateTrusted(false)
				case <-ipOverWriter.tickerQuit:
					ipOverWriter.ticker.Stop()
					return
				}
			}
		}()
	}

	if len(config.ClusterCIDR) > 0 {
		for _, ip := range config.ClusterCIDR {
			_, clusterNet, err := net.ParseCIDR(ip)
			if err != nil {
				return nil, err
			}
			ipOverWriter.clusterNet = append(ipOverWriter.clusterNet, clusterNet)
		}
	}

	return ipOverWriter, nil
}

func (r *RealIPOverWriter) UpdateTrusted(reset bool) {
	if reset {
		r.ticker.Reset(tickerInterval)
	}
	iprecords, err := net.LookupIP(r.dnsName)
	TrustIPNew := []*net.IPNet{}
	if err == nil {
		for _, ip := range iprecords {
			TrustIPNew = append(TrustIPNew, &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)})
		}
	}
	r.TrustIP = TrustIPNew
}

func (r *RealIPOverWriter) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	trustResult := r.trust(req.RemoteAddr, len(req.Header.Get(cfConnectingIP)) > 0)
	if trustResult.isFatal {
		http.Error(rw, "Unknown source", http.StatusInternalServerError)
		return
	}
	if trustResult.isError {
		http.Error(rw, "Unknown source", http.StatusBadRequest)
		return
	}
	if trustResult.directIP == "" {
		http.Error(rw, "Unknown source", http.StatusUnprocessableEntity)
		return
	}
	if trustResult.trusted {
		if req.Header.Get(cfVisitor) != "" {
			var cfVisitorValue CFVisitorHeader
			if err := json.Unmarshal([]byte(req.Header.Get(cfVisitor)), &cfVisitorValue); err != nil {
				req.Header.Set(xCfTrusted, "danger")
				req.Header.Del(cfVisitor)
				req.Header.Del(cfConnectingIP)
				r.next.ServeHTTP(rw, req)
				return
			}
			req.Header.Set(xForwardProto, cfVisitorValue.Scheme)
		}
		if len(req.Header.Get(cfConnectingIP)) > 0 {
			req.Header.Set(xCfTrusted, "yes")
			req.Header.Set(xForwardFor, req.Header.Get(cfConnectingIP))
			req.Header.Set(xRealIP, req.Header.Get(cfConnectingIP))
		}
	} else {
		req.Header.Set(xCfTrusted, "no")
		req.Header.Set(xRealIP, trustResult.directIP)
		req.Header.Del(cfVisitor)
		req.Header.Del(cfConnectingIP)
	}
	r.next.ServeHTTP(rw, req)
}

func (r *RealIPOverWriter) trust(s string, isCF bool) *TrustResult {
	temp, _, err := net.SplitHostPort(s)
	if err != nil {
		return &TrustResult{
			isFatal:  true,
			isError:  true,
			trusted:  false,
			directIP: "",
		}
	}
	ip := net.ParseIP(temp)
	if ip == nil {
		return &TrustResult{
			isFatal:  false,
			isError:  true,
			trusted:  false,
			directIP: "",
		}
	}
	for _, network := range r.TrustIP {
		if network.Contains(ip) {
			return &TrustResult{
				isFatal:  false,
				isError:  false,
				trusted:  true,
				directIP: ip.String(),
			}
		} else if isCF && r.inCluster(ip) {
			r.UpdateTrusted(true)
			if network.Contains(ip) {
				return &TrustResult{
					isFatal:  false,
					isError:  false,
					trusted:  true,
					directIP: ip.String(),
				}
			}
		}
	}
	return &TrustResult{
		isFatal:  false,
		isError:  false,
		trusted:  false,
		directIP: ip.String(),
	}
}

func (r *RealIPOverWriter) inCluster(ip net.IP) bool {
	for _, network := range r.clusterNet {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
