#See This for example
https://github.com/golang/example/blob/master/template/main.go

```go
import (
  "html/template"
  "log"
  "net/http"
  "strings"
)


//global definition, replace static index declaration with this
var indexTemplate = template.Must(template.ParseFiles("index.tmpl"))


//in handler w is http.ResponseWriter
err := imageTemplate.Execute(w, data); 
if err != nil {
  panic(err)
}

```
