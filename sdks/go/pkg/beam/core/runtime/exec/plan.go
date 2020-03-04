// Licensed to the Apache Software Foundation (ASF) under one or more
// contributor license agreements.  See the NOTICE file distributed with
// this work for additional information regarding copyright ownership.
// The ASF licenses this file to You under the Apache License, Version 2.0
// (the "License"); you may not use this file except in compliance with
// the License.  You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package exec contains runtime plan representation and execution. A pipeline
// must be translated to a runtime plan to be executed.
package exec

import (
	"context"
	"fmt"
	"strings"

	"github.com/apache/beam/sdks/go/pkg/beam/internal/errors"
)

// Plan represents the bundle execution plan. It will generally be constructed
// from a part of a pipeline. A plan can be used to process multiple bundles
// serially.
type Plan struct {
	id    string // id of the bundle descriptor for this plan
	roots []Root
	units []Unit
	pcols []*PCollection

	status Status

	// TODO: there can be more than 1 DataSource in a bundle.
	source *DataSource
}

// NewPlan returns a new bundle execution plan from the given units.
func NewPlan(id string, units []Unit) (*Plan, error) {
	var roots []Root
	var pcols []*PCollection
	var source *DataSource

	for _, u := range units {
		if u == nil {
			return nil, errors.Errorf("no <nil> units")
		}
		if r, ok := u.(Root); ok {
			roots = append(roots, r)
		}
		if s, ok := u.(*DataSource); ok {
			source = s
		}
		if p, ok := u.(*PCollection); ok {
			pcols = append(pcols, p)
		}
	}
	if len(roots) == 0 {
		return nil, errors.Errorf("no root units")
	}

	return &Plan{
		id:     id,
		status: Initializing,
		roots:  roots,
		units:  units,
		pcols:  pcols,
		source: source,
	}, nil
}

// ID returns the plan identifier.
func (p *Plan) ID() string {
	return p.id
}

// SourcePTransformID returns the ID of the data's origin PTransform.
func (p *Plan) SourcePTransformID() string {
	return p.source.SID.PtransformID
}

// Execute executes the plan with the given data context and bundle id. Units
// are brought up on the first execution. If a bundle fails, the plan cannot
// be reused for further bundles. Does not panic. Blocking.
func (p *Plan) Execute(ctx context.Context, id string, manager DataContext) error {
	if p.status == Initializing {
		for _, u := range p.units {
			if err := callNoPanic(ctx, u.Up); err != nil {
				p.status = Broken
				return err
			}
		}
		p.status = Up
	}

	if p.status != Up {
		return errors.Errorf("invalid status for plan %v: %v", p.id, p.status)
	}

	// Process bundle. If there are any kinds of failures, we bail and mark the plan broken.

	p.status = Active
	for _, root := range p.roots {
		if err := callNoPanic(ctx, func(ctx context.Context) error { return root.StartBundle(ctx, id, manager) }); err != nil {
			p.status = Broken
			return err
		}
	}
	for _, root := range p.roots {
		if err := callNoPanic(ctx, root.Process); err != nil {
			p.status = Broken
			return err
		}
	}
	for _, root := range p.roots {
		if err := callNoPanic(ctx, root.FinishBundle); err != nil {
			p.status = Broken
			return err
		}
	}

	p.status = Up
	return nil
}

// Down takes the plan and associated units down. Does not panic.
func (p *Plan) Down(ctx context.Context) error {
	if p.status == Down {
		return nil // ok: already down
	}
	p.status = Down

	var errs []error
	for _, u := range p.units {
		if err := callNoPanic(ctx, u.Down); err != nil {
			errs = append(errs, err)
		}
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errors.Wrapf(errs[0], "plan %v failed", p.id)
	default:
		return errors.Errorf("plan %v failed with multiple errors: %v", p.id, errs)
	}
}

func (p *Plan) String() string {
	var units []string
	for _, u := range p.units {
		units = append(units, fmt.Sprintf("%v: %v", u.ID(), u))
	}
	return fmt.Sprintf("Plan[%v]:\n%v", p.ID(), strings.Join(units, "\n"))
}

// PlanSnapshot contains system metrics for the current run of the plan.
type PlanSnapshot struct {
	Source ProgressReportSnapshot
	PCols  []PCollectionSnapshot
}

// Progress returns a snapshot of progress of the plan, and associated metrics. 
// The retuend boolean indicates whether the plan includes a DataSource, which is
// important for handling legacy metrics. This boolean will be removed once
// we no longer return legacy metrics.
func (p *Plan) Progress() (PlanSnapshot, bool) {
	pcolSnaps := make([]PCollectionSnapshot, 0, len(p.pcols)+1) // include space for the datasource pcollection.
	for _, pcol := range p.pcols {
		pcolSnaps = append(pcolSnaps, pcol.snapshot())
	}
	snap := PlanSnapshot{PCols: pcolSnaps}
	if p.source != nil {
		snap.Source = p.source.Progress()
		snap.PCols = append(pcolSnaps, snap.Source.pcol)
		return snap, true
	}
	return snap, false
}

// SplitPoints captures the split requested by the Runner.
type SplitPoints struct {
	// Splits is a list of desired split indices.
	Splits []int64
	Frac   float64
}

// Split takes a set of potential split indexes, and if successful returns
// the split index of the first element of the residual, on which processing
// will be halted.
// Returns an error when unable to split.
func (p *Plan) Split(s SplitPoints) (int64, error) {
	if p.source != nil {
		return p.source.Split(s.Splits, s.Frac)
	}
	return 0, fmt.Errorf("failed to split at requested splits: {%v}, Source not initialized", s)
}