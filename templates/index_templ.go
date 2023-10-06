// Code generated by templ@v0.2.364 DO NOT EDIT.

package templates

//lint:file-ignore SA4006 This context is only used if a nested component is present.

import "github.com/a-h/templ"
import "context"
import "io"
import "bytes"

import (
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
)

func assemblePath(paths []string, i int) string {
	return strings.Join(paths[:i+1], "/")
}

type File struct {
	IsDir       bool
	Path        string
	Dir         string
	Name        string
	Size        uint64
	Description string
	Date        time.Time
	Owner       string
	IsOwner     bool
}

type IndexVars struct {
	Theme     string
	Auth      bool
	User      User
	PathParts []string
	Files     []File
	Path      string
}

func Index(vars IndexVars) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) (err error) {
		templBuffer, templIsBuffer := w.(*bytes.Buffer)
		if !templIsBuffer {
			templBuffer = templ.GetBuffer()
			defer templ.ReleaseBuffer(templBuffer)
		}
		ctx = templ.InitializeContext(ctx)
		var_1 := templ.GetChildren(ctx)
		if var_1 == nil {
			var_1 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		var_2 := templ.ComponentFunc(func(ctx context.Context, w io.Writer) (err error) {
			templBuffer, templIsBuffer := w.(*bytes.Buffer)
			if !templIsBuffer {
				templBuffer = templ.GetBuffer()
				defer templ.ReleaseBuffer(templBuffer)
			}
			err = Main(vars).Render(ctx, templBuffer)
			if err != nil {
				return err
			}
			if !templIsBuffer {
				_, err = io.Copy(w, templBuffer)
			}
			return err
		})
		err = Page(vars.Theme, vars.Auth, vars.User).Render(templ.WithChildren(ctx, var_2), templBuffer)
		if err != nil {
			return err
		}
		if !templIsBuffer {
			_, err = templBuffer.WriteTo(w)
		}
		return err
	})
}

func Main(vars IndexVars) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) (err error) {
		templBuffer, templIsBuffer := w.(*bytes.Buffer)
		if !templIsBuffer {
			templBuffer = templ.GetBuffer()
			defer templ.ReleaseBuffer(templBuffer)
		}
		ctx = templ.InitializeContext(ctx)
		var_3 := templ.GetChildren(ctx)
		if var_3 == nil {
			var_3 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		_, err = templBuffer.WriteString("<main><div id=\"navigation\"><div id=\"navigation-checkbox\"><input type=\"checkbox\" id=\"files-select\" class=\"checkbox\" data-file=\"")
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString(templ.EscapeString(vars.Path))
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("\" autocomplete=\"off\" hx-on:click=\"window.onFilesSelect(event)\"><label for=\"files-select\" class=\"icon\"></label></div><div id=\"navigation-path\"><a href=\"/\">")
		if err != nil {
			return err
		}
		var_4 := `/`
		_, err = templBuffer.WriteString(var_4)
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("</a>")
		if err != nil {
			return err
		}
		for i, path := range vars.PathParts {
			_, err = templBuffer.WriteString("<a href=\"")
			if err != nil {
				return err
			}
			var var_5 templ.SafeURL = templ.URL(assemblePath(vars.PathParts, i))
			_, err = templBuffer.WriteString(templ.EscapeString(string(var_5)))
			if err != nil {
				return err
			}
			_, err = templBuffer.WriteString("\">")
			if err != nil {
				return err
			}
			if i < len(vars.PathParts)-1 {
				var var_6 string = path + "/"
				_, err = templBuffer.WriteString(templ.EscapeString(var_6))
				if err != nil {
					return err
				}
			} else {
				var var_7 string = path
				_, err = templBuffer.WriteString(templ.EscapeString(var_7))
				if err != nil {
					return err
				}
			}
			_, err = templBuffer.WriteString("</a>")
			if err != nil {
				return err
			}
		}
		_, err = templBuffer.WriteString("</div><div class=\"buttons\"><button class=\"btn primary\"")
		if err != nil {
			return err
		}
		if vars.Auth && vars.User.Name == "guest" {
			_, err = templBuffer.WriteString(" disabled")
			if err != nil {
				return err
			}
		}
		_, err = templBuffer.WriteString(" hx-post=\"")
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString(templ.EscapeString(vars.Path + "?action=upload"))
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("\" hx-target=\"main\" hx-swap=\"innerhtml\">")
		if err != nil {
			return err
		}
		var_8 := `Upload`
		_, err = templBuffer.WriteString(var_8)
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("</button><div id=\"files-more\" class=\"dropdown\"><div class=\"icon-btn icon-more\" role=\"button\"></div><ul><li><span role=\"button\" hx-on=\"click: window.onDownloadFiles(event)\">")
		if err != nil {
			return err
		}
		var_9 := `Download`
		_, err = templBuffer.WriteString(var_9)
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("</span></li><li")
		if err != nil {
			return err
		}
		if vars.User.Name == "guest" {
			_, err = templBuffer.WriteString(" disabled")
			if err != nil {
				return err
			}
		}
		_, err = templBuffer.WriteString("><span hx-patch=\"")
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString(templ.EscapeString(vars.Path + "?action=move"))
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("\" hx-target=\"main\" hx-swap=\"innerHTML\" hx-ext=\"move-files,accept-html\" role=\"button\">")
		if err != nil {
			return err
		}
		var_10 := `Move`
		_, err = templBuffer.WriteString(var_10)
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("</span></li><li")
		if err != nil {
			return err
		}
		if vars.User.Name == "guest" {
			_, err = templBuffer.WriteString(" disabled")
			if err != nil {
				return err
			}
		}
		_, err = templBuffer.WriteString("><span hx-delete=\"")
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString(templ.EscapeString(vars.Path))
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("\" hx-target=\"main\" hx-swap=\"outerHTML\" hx-confirm=\"Are you sure you want to delete the selected files?\" hx-ext=\"delete-files,accept-html\" role=\"button\">")
		if err != nil {
			return err
		}
		var_11 := `Delete`
		_, err = templBuffer.WriteString(var_11)
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("</span></li></ul></div></div></div>")
		if err != nil {
			return err
		}
		err = FileList(vars.Auth, vars.Files).Render(ctx, templBuffer)
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("</main>")
		if err != nil {
			return err
		}
		if !templIsBuffer {
			_, err = templBuffer.WriteTo(w)
		}
		return err
	})
}

func FileList(auth bool, files []File) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) (err error) {
		templBuffer, templIsBuffer := w.(*bytes.Buffer)
		if !templIsBuffer {
			templBuffer = templ.GetBuffer()
			defer templ.ReleaseBuffer(templBuffer)
		}
		ctx = templ.InitializeContext(ctx)
		var_12 := templ.GetChildren(ctx)
		if var_12 == nil {
			var_12 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		var var_13 = []any{"table-list", templ.KV("auth", auth)}
		err = templ.RenderCSSItems(ctx, templBuffer, var_13...)
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("<div id=\"file-list\" class=\"")
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString(templ.EscapeString(templ.CSSClasses(var_13).String()))
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("\"><div class=\"table-list-header\"><div></div><div></div><div>")
		if err != nil {
			return err
		}
		var_14 := `Name`
		_, err = templBuffer.WriteString(var_14)
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("</div><div>")
		if err != nil {
			return err
		}
		var_15 := `Size`
		_, err = templBuffer.WriteString(var_15)
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("</div><div>")
		if err != nil {
			return err
		}
		var_16 := `Last Edited`
		_, err = templBuffer.WriteString(var_16)
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("</div><div>")
		if err != nil {
			return err
		}
		var_17 := `Description`
		_, err = templBuffer.WriteString(var_17)
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("</div><div>")
		if err != nil {
			return err
		}
		var_18 := `Owner`
		_, err = templBuffer.WriteString(var_18)
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("</div><div></div></div>")
		if err != nil {
			return err
		}
		for i, file := range files {
			err = FileEntry(auth, i, file).Render(ctx, templBuffer)
			if err != nil {
				return err
			}
		}
		_, err = templBuffer.WriteString("</div>")
		if err != nil {
			return err
		}
		if !templIsBuffer {
			_, err = templBuffer.WriteTo(w)
		}
		return err
	})
}

func FileEntry(auth bool, i int, file File) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) (err error) {
		templBuffer, templIsBuffer := w.(*bytes.Buffer)
		if !templIsBuffer {
			templBuffer = templ.GetBuffer()
			defer templ.ReleaseBuffer(templBuffer)
		}
		ctx = templ.InitializeContext(ctx)
		var_19 := templ.GetChildren(ctx)
		if var_19 == nil {
			var_19 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		_, err = templBuffer.WriteString("<div class=\"table-list-entry\"><div>")
		if err != nil {
			return err
		}
		var var_20 = []any{"file-select", "checkbox", templ.KV("folder", file.IsDir), templ.KV("file", !file.IsDir)}
		err = templ.RenderCSSItems(ctx, templBuffer, var_20...)
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("<input type=\"checkbox\" id=\"")
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString(templ.EscapeString("file-select-" + strconv.Itoa(i)))
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("\" class=\"")
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString(templ.EscapeString(templ.CSSClasses(var_20).String()))
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("\" data-name=\"")
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString(templ.EscapeString(file.Name))
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("\" autocomplete=\"off\" hx-on:click=\"window.onFileSelect(event)\"><label for=\"")
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString(templ.EscapeString("file-select-" + strconv.Itoa(i)))
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("\" class=\"icon\"></label></div><div>")
		if err != nil {
			return err
		}
		var var_21 = []any{"icon", templ.KV("icon-folder", file.IsDir), templ.KV("icon-file", !file.IsDir)}
		err = templ.RenderCSSItems(ctx, templBuffer, var_21...)
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("<span class=\"")
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString(templ.EscapeString(templ.CSSClasses(var_21).String()))
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("\"></span></div><div hx-boost=\"")
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString(templ.EscapeString(strconv.FormatBool(file.IsDir)))
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("\"><a href=\"")
		if err != nil {
			return err
		}
		var var_22 templ.SafeURL = templ.URL(file.Path)
		_, err = templBuffer.WriteString(templ.EscapeString(string(var_22)))
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("\">")
		if err != nil {
			return err
		}
		var var_23 string = file.Name
		_, err = templBuffer.WriteString(templ.EscapeString(var_23))
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("</a></div><div>")
		if err != nil {
			return err
		}
		var var_24 string = humanize.IBytes(file.Size)
		_, err = templBuffer.WriteString(templ.EscapeString(var_24))
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("</div><div title=\"")
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString(templ.EscapeString(file.Date.Format(time.DateTime)))
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("\">")
		if err != nil {
			return err
		}
		var var_25 string = humanize.Time(file.Date)
		_, err = templBuffer.WriteString(templ.EscapeString(var_25))
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("</div><div>")
		if err != nil {
			return err
		}
		var var_26 string = file.Description
		_, err = templBuffer.WriteString(templ.EscapeString(var_26))
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("</div><div>")
		if err != nil {
			return err
		}
		var var_27 string = file.Owner
		_, err = templBuffer.WriteString(templ.EscapeString(var_27))
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("</div><div><div class=\"dropdown\"><div class=\"icon-btn icon-more\" role=\"button\"></div><ul><li><a href=\"")
		if err != nil {
			return err
		}
		var var_28 templ.SafeURL = templ.URL(file.Path + "?dl=1")
		_, err = templBuffer.WriteString(templ.EscapeString(string(var_28)))
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("\" target=\"_blank\">")
		if err != nil {
			return err
		}
		var_29 := `Download`
		_, err = templBuffer.WriteString(var_29)
		if err != nil {
			return err
		}
		_, err = templBuffer.WriteString("</a></li>")
		if err != nil {
			return err
		}
		if file.IsOwner {
			if file.IsDir {
				_, err = templBuffer.WriteString("<li><span hx-patch=\"")
				if err != nil {
					return err
				}
				_, err = templBuffer.WriteString(templ.EscapeString(file.Path + "?action=move"))
				if err != nil {
					return err
				}
				_, err = templBuffer.WriteString("\" hx-target=\"main\" hx-swap=\"innerHTML\" role=\"button\">")
				if err != nil {
					return err
				}
				var_30 := `Move`
				_, err = templBuffer.WriteString(var_30)
				if err != nil {
					return err
				}
				_, err = templBuffer.WriteString("</span></li>")
				if err != nil {
					return err
				}
			} else {
				_, err = templBuffer.WriteString("<li><span hx-patch=\"")
				if err != nil {
					return err
				}
				_, err = templBuffer.WriteString(templ.EscapeString(file.Path + "?action=edit"))
				if err != nil {
					return err
				}
				_, err = templBuffer.WriteString("\" hx-target=\"main\" hx-swap=\"innerHTML\" role=\"button\">")
				if err != nil {
					return err
				}
				var_31 := `Edit`
				_, err = templBuffer.WriteString(var_31)
				if err != nil {
					return err
				}
				_, err = templBuffer.WriteString("</span></li>")
				if err != nil {
					return err
				}
			}
			_, err = templBuffer.WriteString(" <li><span hx-delete=\"")
			if err != nil {
				return err
			}
			_, err = templBuffer.WriteString(templ.EscapeString(file.Path))
			if err != nil {
				return err
			}
			_, err = templBuffer.WriteString("\" hx-target=\"main\" hx-swap=\"outerHTML\" hx-confirm=\"")
			if err != nil {
				return err
			}
			_, err = templBuffer.WriteString(templ.EscapeString("Are you sure you want to delete " + file.Name + "?"))
			if err != nil {
				return err
			}
			_, err = templBuffer.WriteString("\" hx-ext=\"accept-html\" role=\"button\">")
			if err != nil {
				return err
			}
			var_32 := `Delete`
			_, err = templBuffer.WriteString(var_32)
			if err != nil {
				return err
			}
			_, err = templBuffer.WriteString("</span></li>")
			if err != nil {
				return err
			}
		}
		_, err = templBuffer.WriteString("</ul></div></div></div>")
		if err != nil {
			return err
		}
		if !templIsBuffer {
			_, err = templBuffer.WriteTo(w)
		}
		return err
	})
}
