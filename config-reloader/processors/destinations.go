// Copyright © 2018 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: BSD-2-Clause

package processors

import (
	"errors"
	"fmt"

	"github.com/vmware/kube-fluentd-operator/config-reloader/fluentd"
	"github.com/vmware/kube-fluentd-operator/config-reloader/util"
)

const (
	paramBufferPath = "buffer_path"
)

type fixDestinations struct {
	BaseProcessorState
}

func makeSafeBufferPath(ctx *ProcessorContext, origBufPath string) string {
	// make a custom buffer path directory if BufferMountFolder is set:
	if ctx.BufferMountFolder != "" {
		return fmt.Sprintf("/var/log/%s/kfo-%s-%s-%s.buf", ctx.BufferMountFolder, util.MakeFluentdSafeName(ctx.DeploymentID), ctx.Namespace, util.Hash("", origBufPath))
	}
	return fmt.Sprintf("/var/log/kfo-%s-%s-%s.buf", util.MakeFluentdSafeName(ctx.DeploymentID), ctx.Namespace, util.Hash("", origBufPath))
}

func prohibitSources(d *fluentd.Directive, ctx *ProcessorContext) error {
	if d.Name == "source" {
		if d.Type() != mountedFileSourceType {
			return errors.New("cannot use <source> directive")
		}
	}

	return nil
}

func prohibitTypes(d *fluentd.Directive, ctx *ProcessorContext) error {
	if d.Name != "match" &&
		d.Name != "store" &&
		d.Name != "filter" {
		return nil
	}

	switch d.Type() {
	case "exec", "exec_filter",
		"stdout", "rewrite_tag_filter":
		return fmt.Errorf("cannot use '@type %s' in <%s>", d.Type(), d.Name)
	case "detect_exceptions":
		if d.Name == "match" {
			return fmt.Errorf("cannot use '@type %s' in <%s>", d.Type(), d.Name)
		}
	case "file":
		if !ctx.AllowFile {
			return fmt.Errorf("cannot use '@type %s' in <%s>", d.Type(), d.Name)
		}
	case "mounted-file":
		if !ctx.AllowMountedFile {
			return fmt.Errorf("cannot use '@type %s' in <%s>", d.Type(), d.Name)
		}
	case "fields_parser":
		if d.Param("remove_tag_prefix") != "" ||
			d.Param("add_tag_prefix") != "" {
			return fmt.Errorf("cannot modify tags using the plugin %s", d.Type())
		}
	}

	return nil
}

func rewriteBufferPath(d *fluentd.Directive, ctx *ProcessorContext) error {
	if d.Name == "match" || d.Name == "store" {
		origBufPath := d.Param(paramBufferPath)
		if origBufPath != "" {
			d.SetParam(paramBufferPath, makeSafeBufferPath(ctx, origBufPath))
		}
		return nil
	}

	if d.Name == "buffer" && d.Type() == "file" {
		path := d.Param("path")
		if path != "" {
			d.SetParam("path", makeSafeBufferPath(ctx, path))
		}
	}

	return nil
}

func (p *fixDestinations) Process(input fluentd.Fragment) (fluentd.Fragment, error) {
	funcs := []func(*fluentd.Directive, *ProcessorContext) error{
		prohibitTypes,
		rewriteBufferPath,
		prohibitSources,
	}

	for _, f := range funcs {
		err := applyRecursivelyInPlace(input, p.Context, f)
		if err != nil {
			return nil, err
		}
	}

	return input, nil
}
