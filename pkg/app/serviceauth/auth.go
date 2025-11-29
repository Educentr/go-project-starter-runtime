package serviceauth

import (
	"github.com/Educentr/go-project-starter-runtime/pkg/app"
	"github.com/Educentr/go-project-starter-runtime/pkg/ds"
)

type Authorizer struct {
	app.UnimplementedAuthorizer
}

func (a *Authorizer) GetAuthorizer() ds.Authorizer {
	return a
}
