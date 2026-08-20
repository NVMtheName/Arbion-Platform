package mailer

import (
	"bytes"
	"html/template"
)

type BrandedEmailContent struct {
	Preheader   string
	LogoURL     string
	Heading     string
	Intro       string
	ActionLabel string
	ActionURL   string
	Detail      string
}

var brandedEmailTemplate = template.Must(template.New("arbion-email").Parse(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>{{.Heading}}</title>
  </head>
  <body style="margin:0;padding:0;background:#07110e;color:#edf7f1;font-family:Inter,Arial,sans-serif;">
    <div style="display:none;max-height:0;overflow:hidden;opacity:0;color:transparent;">{{.Preheader}}</div>
    <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="background:#07110e;padding:32px 16px;">
      <tr>
        <td align="center">
          <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="max-width:560px;background:#0d211a;border:1px solid #244237;border-radius:20px;overflow:hidden;box-shadow:0 24px 64px rgba(0,0,0,.35);">
            <tr><td style="height:5px;background:#19c9d6;"></td></tr>
            <tr>
              <td style="padding:34px 38px 14px;">
                <img src="{{.LogoURL}}" width="176" alt="Arbion" style="display:block;width:176px;max-width:58%;height:auto;border:0;">
              </td>
            </tr>
            <tr>
              <td style="padding:10px 38px 38px;">
                <h1 style="margin:0 0 16px;color:#edf7f1;font-size:30px;line-height:1.2;">{{.Heading}}</h1>
                <p style="margin:0 0 24px;color:#cfe4da;font-size:16px;line-height:1.65;">{{.Intro}}</p>
                <table role="presentation" cellspacing="0" cellpadding="0" style="margin:0 0 24px;">
                  <tr>
                    <td style="border-radius:10px;background:#5ee0a0;">
                      <a href="{{.ActionURL}}" style="display:inline-block;padding:14px 22px;color:#062116;font-size:16px;font-weight:700;text-decoration:none;">{{.ActionLabel}}</a>
                    </td>
                  </tr>
                </table>
                <p style="margin:0 0 18px;color:#9fc1b2;font-size:14px;line-height:1.6;">{{.Detail}}</p>
                <p style="margin:0 0 8px;color:#7fa596;font-size:12px;line-height:1.5;">If the button does not work, copy this secure link into your browser:</p>
                <p style="margin:0;word-break:break-all;font-size:12px;line-height:1.5;"><a href="{{.ActionURL}}" style="color:#19c9d6;">{{.ActionURL}}</a></p>
              </td>
            </tr>
            <tr>
              <td style="padding:20px 38px;border-top:1px solid #244237;color:#7fa596;font-size:12px;line-height:1.5;">
                Arbion · Secure, disciplined financial decisions
              </td>
            </tr>
          </table>
        </td>
      </tr>
    </table>
  </body>
</html>`))

func RenderBrandedHTML(content BrandedEmailContent) (string, error) {
	var output bytes.Buffer
	if err := brandedEmailTemplate.Execute(&output, content); err != nil {
		return "", err
	}
	return output.String(), nil
}
