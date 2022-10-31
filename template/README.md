#See This for example
https://github.com/golang/example/blob/master/template/main.go

```go
import (
	"html/template"
	"log"
	"net/http"
	"strings"
)


//global definition
var indexTemplate = template.Must(template.ParseFiles("index.tmpl"))



//in handler w is http.ResponseWriter
if err := imageTemplate.Execute(w, data); 
  err != nil {
		panic(err)
	}

```
