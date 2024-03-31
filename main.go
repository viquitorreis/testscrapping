package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/url"
	"time"

	"github.com/anthdm/hollywood/actor"
	"github.com/valyala/fasthttp"
	"golang.org/x/net/html"
)

type VisitFunc func(io.Reader) error

type VisitRequest struct {
	links    []string
	visitsFn VisitFunc
}

func NewVisitRequest(links []string) VisitRequest {
	return VisitRequest{
		links: links,
		visitsFn: func(r io.Reader) error {
			fmt.Println("=====================")

			b, err := io.ReadAll(r)
			if err != nil {
				return err
			}
			fmt.Println(string(b))

			fmt.Println("=====================")
			return nil
		},
	}
}

type Visitor struct {
	URL        *url.URL
	managerPID *actor.PID
	visitorFn  VisitFunc
}

func NewVisitor(url *url.URL, mpid *actor.PID, visitFn VisitFunc) actor.Producer {
	return func() actor.Receiver {
		return &Visitor{
			URL:        url,
			managerPID: mpid,
			visitorFn:  visitFn,
		}
	}
}

func (v *Visitor) Receive(c *actor.Context) {
	switch c.Message().(type) {
	case actor.Started:
		slog.Info("Visitor started", "url", v.URL)
		links, err := v.doVisit(v.URL.String(), v.visitorFn)
		if err != nil {
			slog.Error("visitor failed to visit", "url", v.URL, "error", err)
			return
		}
		c.Send(v.managerPID, NewVisitRequest(links))
		c.Engine().Poison(c.PID())

		fmt.Println(links)
	case actor.Stopped:
		fmt.Println("Actor stopped", "url", v.URL)
	}
}

func (v *Visitor) extractLinks(body io.Reader) ([]string, error) {
	links := make([]string, 0)
	tokenizer := html.NewTokenizer(body)

	for {
		tokenType := tokenizer.Next()
		if tokenType == html.ErrorToken {
			return links, nil
		}

		if tokenType == html.StartTagToken {
			token := tokenizer.Token()
			if token.Data == "a" {
				for _, attr := range token.Attr {
					if attr.Key == "href" {
						// if attr.Val[0] == '#' { // com hashtags vai só continuar
						// 	continue
						// }
						lurl, err := url.Parse(attr.Val)
						if err != nil {
							return links, err
						}
						actualLink := v.URL.ResolveReference(lurl)

						links = append(links, actualLink.String())
					}
				}
			}
		}
	}
}

func (v *Visitor) doVisit(link string, visitFn VisitFunc) ([]string, error) {
	baseUrl, err := url.Parse(link)
	if err != nil {
		return []string{}, err
	}

	statusCode, body, err := Get([]byte{}, baseUrl.String())
	if err != nil {
		return []string{}, err
	}

	if statusCode != 200 {
		log.Fatal("Failed to fetch URL")
	}

	// reader, err := gzip.NewReader(bytes.NewReader(body))
	// if err != nil {
	// 	return []string{}, err
	// }
	// defer reader.Close()

	// decompressedBody, err := io.ReadAll(io.Reader(bytes.NewReader(body)))
	// if err != nil {
	// 	return []string{}, err
	// }

	bodyReader := bytes.NewReader(body)
	w := &bytes.Buffer{}
	r := io.TeeReader(bodyReader, w)

	// ARRUMAR DEPOIS
	links, err := v.extractLinks(r)
	if err != nil {
		return []string{}, nil
	}

	if err := visitFn(w); err != nil {
		return []string{}, err
	}

	return links, nil
}

type Manager struct {
	visited  map[string]bool
	pegasus  *Pegasus
	visitors map[*actor.PID]bool
}

func NewManager() actor.Producer {
	return func() actor.Receiver {
		return &Manager{
			pegasus:  Default(), // ARRUMAR
			visitors: make(map[*actor.PID]bool),
			visited:  make(map[string]bool),
		}
	}
}

func (m *Manager) Receive(c *actor.Context) {
	switch msg := c.Message().(type) {
	case VisitRequest:
		m.handleVisitRequest(c, msg)
	case actor.Started:
		slog.Info("Hello world! Actor started")
		_ = msg
	case actor.Stopped:
		fmt.Println("Actor stopped")
	}
}

func (m *Manager) handleVisitRequest(c *actor.Context, msg VisitRequest) error {
	for _, link := range msg.links {
		if _, ok := m.visited[link]; !ok {
			slog.Info("Visiting link: ", "url", link)
			baseURL, err := url.Parse(link)
			if err != nil {
				return err
			}
			c.SpawnChild(NewVisitor(baseURL, c.PID(), msg.visitsFn), "visitor/"+link)
			m.visited[link] = true
		}
	}
	return nil
}

// for _, link := range extractedLinks {
// 	endUrl, err := url.Parse(link)
// 	if err != nil {
// 		return links, err
// 	}

// 	actualLink := baseUrl.ResolveReference(endUrl) // vai resolver a URL com base em um absolute
// 	fmt.Println(actualLink)
// }

// func (p *Pegasus) NewPegasus() {

// }

// type PegasusColletor struct {
// 	pegasus *Pegasus
// }

func main() {
	baseUrl := "https://crawler-test.com"
	// baseUrl := "https://books.toscrape.com"
	// p := Default()
	// p.doVisit(baseUrl)

	e, err := actor.NewEngine(actor.NewEngineConfig())
	if err != nil {
		log.Fatal(err)
	}

	pid := e.Spawn(NewManager(), "manager")

	time.Sleep(time.Millisecond * 200)
	e.Send(pid, NewVisitRequest([]string{baseUrl}))

	time.Sleep(time.Second * 1000) // só para rodar a fins de teste
}

// PEGASUS + HTTPFAST
func Get(dst []byte, url string) (statusCode int, body []byte, err error) {
	req := fasthttp.AcquireRequest()
	req.Header = fasthttp.RequestHeader{}

	// Headers.CopyTo(&req.Header)
	// req.Header.Set("Accept-Encoding", "gzip, deflate, br, sdch")

	req.SetRequestURI(url)

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// contentEncoding := resp.Header.Peek("Content-Encoding")
	// switch string(contentEncoding) {
	// case "gzip":
	// 	reader, err := gzip.NewReader(bytes.NewReader(resp.Body()))
	// 	if err != nil {
	// 		return 0, nil, err
	// 	}
	// 	defer reader.Close()

	// 	fmt.Println(reader)
	// case "deflate":
	// 	// reader, err := flate.NewReader(bytes.NewReader(resp.Body()), flate.DefaultCompression)
	// 	fmt.Println("deflate")
	// case "br":
	// 	fmt.Println("br")
	// default:
	// 	fmt.Println("default")

	// }

	// for _, key := range p.Headers.PeekKeys() {
	// 	value := req.Header.Peek(string(key))
	// 	fmt.Println(value)

	// 	expectedValue := Headers.Peek(string(key))
	// 	if !bytes.Equal(value, expectedValue) {
	// 		return 0, nil, fmt.Errorf("header %s was not set correctly", key)
	// 	}
	// }

	if err := fasthttp.Do(req, resp); err != nil {
		return 0, nil, err
	}

	statusCode = resp.StatusCode()
	body = resp.Body()

	return statusCode, body, nil
}

// container de configuração para o Pegasus
type Options struct {
	UserAgent string
	Headers   *fasthttp.RequestHeader
}

// Handler do Pegasus
type Pegasus struct {
	UserAgent string
	Headers   *fasthttp.RequestHeader
}

func NewPegasus(options Options) *Pegasus {
	p := &Pegasus{
		UserAgent: options.UserAgent,
		Headers:   options.Headers,
	}

	if p.UserAgent == "" {
		p.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/"
	}

	p.Headers = &fasthttp.RequestHeader{}
	p.Headers.Add("User-Agent", p.UserAgent)

	// ARRUMAR CONDIÇÕES CORRETAMENTE
	if p.Headers == nil {
		p.Headers = &fasthttp.RequestHeader{}
		p.Headers.Add("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
		// p.Headers.Add("Accept-Encoding", "gzip, deflate, br, sdch")
		p.Headers.Add("Accept-Language", "en-US,en;q=0.8")
		p.Headers.Add("Referer", "https://www.google.com/")
	}

	return p
}

func Default() *Pegasus {
	return NewPegasus(Options{})
}
