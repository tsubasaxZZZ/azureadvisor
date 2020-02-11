package main

import (
	"os"
	"text/template"
)

func outputToHTML(data interface{}, outpuFilePath string, templateName string) error {
	file, err := os.Create(outpuFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	tpl := template.Must(template.ParseFiles(templateName))

	if err = tpl.ExecuteTemplate(file, templateName, data); err != nil {
		return err
	}

	return nil
}
