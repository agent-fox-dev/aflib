package p

import (
	"fmt"
	"net/http"

	"example.com/m/other"
	"github.com/google/uuid"
)

var _ = fmt.Sprint(http.StatusOK, other.X, uuid.Nil)
