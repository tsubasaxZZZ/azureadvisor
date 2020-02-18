package main

import (
	"io/ioutil"
	"os"
	"strings"
	"text/template"
	"time"

	_ "azureadvisor/statik"

	"github.com/rakyll/statik/fs"
)

//go:generate statik -f -src tmpl

func outputToFile(data interface{}, outputFilePath string, templateName string) error {
	file, err := os.Create(outputFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	statikFs, err := fs.New()
	if err != nil {
		return err
	}

	// ----- コンテンツテンプレート
	templateFile, err := statikFs.Open("/" + templateName)
	if err != nil {
		return err
	}
	templateBytes, err := ioutil.ReadAll(templateFile)
	if err != nil {
		return err
	}
	// --------------

	funcs := template.FuncMap{
		"add": func(x, y int) int {
			return x + y
		},
	}
	tpl := template.Must(template.New(templateName).Funcs(funcs).Parse(string(templateBytes)))

	// HTML テンプレートを指定された場合
	if strings.Index(templateName, ".html") > -1 {

		// ----- ヘッダーテンプレート
		headerTemplateFile, err := statikFs.Open("/header.tmpl.html")
		if err != nil {
			return err
		}
		headerTemplateBytes, err := ioutil.ReadAll(headerTemplateFile)
		if err != nil {
			return err
		}
		// --------------

		// ----- 共通テンプレート
		infoTemplateFile, err := statikFs.Open("/information.tmpl.html")
		if err != nil {
			return err
		}
		infoTemplateBytes, err := ioutil.ReadAll(infoTemplateFile)
		if err != nil {
			return err
		}
		// --------------
		tplInformation := template.Must(template.New("information").Funcs(funcs).Parse(string(infoTemplateBytes)))
		tplHeader := template.Must(template.New("information").Funcs(funcs).Parse(string(headerTemplateBytes)))
		tpl.AddParseTree("information", tplInformation.Tree)
		tpl.AddParseTree("header", tplHeader.Tree)
	}

	info := map[string]interface{}{
		"createdDate": time.Now().Format("2006-01-02 15:04:05"),
	}
	d := map[string]interface{}{
		"Data": data,
		"Info": info,
	}

	if err = tpl.ExecuteTemplate(file, templateName, d); err != nil {
		return err
	}

	return nil
}
