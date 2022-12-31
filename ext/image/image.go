package myproxy_image

import (
	"image"
	"net/http"

	. "github.com/fj9140/myproxy"
)

var RespIsImage = ContentTypeIs(
	"image/gif",
	"image/jpeg",
	"image/pjpeg",
	"application/octet-stream",
	"image/png")

func HandleImage(f func(img image.Image, ctx *ProxyCtx) image.Image) RespHandler {
	return FuncRespHandler(func(resp *http.Response, ctx *ProxyCtx) *http.Response {
		if !RespIsImage.HandleResp(resp, ctx) {
			return resp
		}
		if resp.StatusCode != 200 {
			return resp
		}
		contentType := resp.Header.Get("Content-Type")

		const kb = 1024

		return resp
	})
}
