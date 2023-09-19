package plugin

import (
	"net/http"
)

/*

todo

	熔断降级

	graceful

	dashboard

*/

type Gateway struct {
	router  *Router
	limiter *Limiter
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := g.limiter.Limit(w, r); err != nil {
		JsonResponseError(w, err.Error())
		return
	}
	g.router.ReverseProxy(w, r)
}

func NewGateway() *Gateway {
	g := &Gateway{
		router:  NewRouter(),
		limiter: NewLimiter(10, 0),
	}

	return g
}
