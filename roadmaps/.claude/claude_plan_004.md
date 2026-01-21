Codepicker Enhancement Roadmap

Version: 1.0

Created: January 2026

Status: Planning Phase

📋 Executive Summary

This roadmap outlines architectural improvements and new features for Codepicker to enhance autonomy, cost efficiency, and developer experience. The focus is on adding planning capabilities, batch job management, and improved model orchestration.

🎯 Phase 1: Foundation & Planning (Weeks 1-2)

1.1 Planning System Implementation

Priority: 🔥 Critical

Effort: Medium (3-5 days)

Objectives

Add AI-driven task planning before execution

Provide cost/time estimates upfront

Enable plan review and approval workflow

Deliverables

internal/agent/

├── planner.go # NEW: AI-based task planning

├── plan_types.go # NEW: Plan data structures

└── plan_executor.go # NEW: Execute plans with checkpointing

Implementation Steps

Create Planner struct with CreatePlan() method

Add JSON schema for plan output format

Implement plan cost/time estimation

Add codepicker plan <task> CLI command

Integrate plan approval into existing do command

Success Metrics

Plans generated in <5 seconds

Cost estimates within 20% accuracy

User can approve/reject before execution

1.2 Database Schema Extensions

Priority: 🔥 Critical

Effort: Small (1-2 days)

Objectives

Support batch job queuing

Track plan execution history

Store agent performance metrics

Schema Changes

-- Add to internal/database/store.go



CREATE TABLE IF NOT EXISTS jobs (

id TEXT PRIMARY KEY,

task TEXT NOT NULL,

priority INTEGER DEFAULT 0,

status TEXT NOT NULL, -- pending/running/completed/failed

plan_json TEXT,

result TEXT,

error TEXT,

cost_usd REAL DEFAULT 0.0,

tokens_used INTEGER DEFAULT 0,

created_at DATETIME DEFAULT CURRENT_TIMESTAMP,

started_at DATETIME,

completed_at DATETIME

);



CREATE TABLE IF NOT EXISTS plans (

id TEXT PRIMARY KEY,

task TEXT NOT NULL,

steps_json TEXT NOT NULL,

estimated_cost REAL,

estimated_turns INTEGER,

actual_cost REAL,

actual_turns INTEGER,

created_at DATETIME DEFAULT CURRENT_TIMESTAMP

);



CREATE INDEX idx_jobs_status ON jobs(status);

CREATE INDEX idx_jobs_priority ON jobs(priority DESC, created_at ASC);

🚀 Phase 2: Batch Job System (Weeks 3-4)

2.1 Batch Queue Infrastructure

Priority: 🔥 High

Effort: Large (5-7 days)

Objectives

Process multiple tasks concurrently

Handle failures gracefully with retries

Provide progress monitoring

Deliverables

internal/batch/

├── queue.go # Job queue management

├── worker.go # Concurrent job processor

├── scheduler.go # Priority-based scheduling

└── persistence.go # Job state persistence



cmd/

├── batch.go # Main batch command

├── batch_add.go # Add jobs from file

├── batch_run.go # Process queue

└── batch_status.go # Monitor progress

Implementation Steps

Implement BatchQueue with SQLite backing

Add semaphore-based concurrency control

Create YAML/JSON task file parser

Build status dashboard (CLI table view)

Add retry logic with exponential backoff

Implement progress persistence (resume on crash)

Configuration

# batch-config.yml

batch:

max_concurrent: 3

retry_attempts: 2

timeout_per_job: 10m

priority_weights:

high: 10

medium: 5

low: 1

2.2 CLI Commands

Priority: 🟡 Medium

Effort: Medium (3-4 days)

New Commands

# Add jobs from file

codepicker batch add tasks.yml



# Process queue with 3 workers

codepicker batch run --concurrent 3



# Monitor status

codepicker batch status



# Cancel running jobs

codepicker batch cancel --job-id JOB_123



# View job logs

codepicker batch logs --job-id JOB_123



# Clear completed jobs

codepicker batch clean --older-than 7d

🏗️ Phase 3: Architecture Refactoring (Weeks 5-6)

3.1 Dependency Injection

Priority: 🟡 Medium

Effort: Medium (4-5 days)

Objectives

Improve testability

Enable component swapping

Reduce tight coupling

Refactoring Plan

// internal/agent/deps.go

type EngineDependencies struct {

Client *openrouter.Client

Model string

Memory *WorkingMemory

Shadow *shadow.Manager

Sentinel *Sentinel

Tracker *tracking.CostTracker

Logger logger.Logger

Config *config.Limits

}



// internal/agent/engine.go (simplified)

func NewEngine(deps EngineDependencies) (*Engine, error) {

return &Engine{

client: deps.Client,

planner: NewPlanner(deps.Client, deps.Model),

executor: NewExecutor(deps.Config),

// ... remaining fields

}, nil

}

Benefits

Easier unit testing with mocks

Clear dependency graph

Better IDE autocomplete

3.2 Code Organization

Priority: 🟢 Low

Effort: Small (2-3 days)

File Splitting

Split monolithic engine.go (currently 200+ lines):



internal/agent/

├── engine.go # Core orchestration (50 lines)

├── planner.go # Planning logic

├── executor.go # Execution logic

├── tools.go # Tool definitions (existing)

├── prompts.go # Prompts (existing)

├── memory.go # Working memory (existing)

├── recovery.go # Recovery strategies (existing)

├── sentinel.go # Command validation (existing)

└── batch_runner.go # Batch execution

💰 Phase 4: Cost Optimization (Weeks 7-8)

4.1 Multi-Model Strategy

Priority: 🔥 High

Effort: Medium (4-5 days)

Objectives

Use cheaper models for simple tasks

Route to expensive models only when needed

Implement model fallback chains

Implementation

// internal/agent/router.go

type ModelRouter struct {

models map[TaskComplexity]string

}



type TaskComplexity int

const (

Simple TaskComplexity = iota // Gemini Flash Lite

Medium // DeepSeek V3

Complex // Claude Sonnet/DeepSeek V3.2

)



func (r *ModelRouter) SelectModel(task string) string {

complexity := r.AnalyzeComplexity(task)

return r.models[complexity]

}

Routing Rules

Task TypeModelCost (per 1M tokens)Simple queriesGemini 2.0 Flash Lite$0.075 / $0.30Code generationDeepSeek V3$0.14 / $0.28Complex refactoringDeepSeek V3.2$0.24 / $0.38Architecture designClaude Sonnet 4.5Market rate4.2 Caching Layer

Priority: 🟡 Medium

Effort: Small (2-3 days)

Objectives

Cache common responses

Reduce redundant API calls

Track cache hit rate

Implementation

// internal/cache/response_cache.go

type ResponseCache struct {

store *database.Store

ttl time.Duration

}



func (c *ResponseCache) Get(prompt string) (string, bool) {

hash := sha256.Sum256([]byte(prompt))

// Check database for cached response

}



func (c *ResponseCache) Set(prompt, response string) {

// Store with TTL

}

📊 Phase 5: Observability & Monitoring (Weeks 9-10)

5.1 Enhanced Metrics

Priority: 🟡 Medium

Effort: Medium (3-4 days)

New Metrics

# Agent Performance

codepicker_agent_turns_total

codepicker_agent_success_rate

codepicker_agent_avg_duration_seconds



# Cost Tracking

codepicker_cost_by_model_usd

codepicker_tokens_by_operation



# Batch Jobs

codepicker_batch_jobs_pending

codepicker_batch_jobs_running

codepicker_batch_jobs_failed

Dashboard

Prometheus-compatible /metrics endpoint (already exists)

Grafana dashboard template

Cost breakdown by model/operation

5.2 Structured Logging

Priority: 🟢 Low

Effort: Small (1-2 days)

Enhancements

// Add context to all log entries

logger.InfoWithContext(ctx, "Agent started", map[string]interface{}{

"task_id": taskID,

"model": model,

"estimated_cost": cost,

})

🧪 Phase 6: Testing & Quality (Weeks 11-12)

6.1 Test Coverage Increase

Priority: 🔥 Critical

Effort: Large (7-10 days)

Coverage Goals

PackageCurrentTargetinternal/agent~20%75%internal/batch0%70%internal/planner0%70%pkg/openrouter0%60%Overall~30%70%Test Types

Unit Tests - All new packages

Integration Tests - Agent workflows

E2E Tests - Full CLI commands

Load Tests - Batch processing

6.2 CI/CD Pipeline

Priority: 🟡 Medium

Effort: Medium (3-4 days)

GitHub Actions Workflow

# .github/workflows/ci.yml

name: CI



on: [push, pull_request]



jobs:

test:

runs-on: ubuntu-latest

steps:

- uses: actions/checkout@v3

- uses: actions/setup-go@v4

with:

go-version: '1.23'


- name: Run tests

run: go test -v -race -coverprofile=coverage.out ./...


- name: Lint

run: golangci-lint run


- name: Security scan

run: gosec ./...

🎨 Phase 7: Developer Experience (Weeks 13-14)

7.1 Documentation

Priority: 🔥 High

Effort: Medium (4-5 days)

Deliverables

Comprehensive README

Quick start guide

Installation instructions

Usage examples

Architecture Documentation

System design overview

Supervisor-Worker pattern explanation

Data flow diagrams

API Reference

Server endpoints

Request/response schemas

Authentication guide

Contribution Guide

Development setup

Code style guidelines

PR process

7.2 Example Projects

Priority: 🟡 Medium

Effort: Small (2-3 days)

Examples

examples/

├── test-generation/

│ ├── README.md

│ └── tasks.yml

├── refactoring/

│ ├── README.md

│ └── before-after/

├── documentation/

│ └── auto-doc-generation.md

└── batch-workflows/

└── ci-integration.yml

📈 Phase 8: Performance Optimization (Weeks 15-16)

8.1 Benchmarking

Priority: 🟢 Low

Effort: Small (2-3 days)

Benchmark Targets

Scanner performance: >1000 files/sec

Context generation: <500ms for 100KB

Minification: <100ms per file

Database queries: <10ms average

8.2 Parallel Processing

Priority: 🟡 Medium

Effort: Medium (3-4 days)

Optimizations

Parallel file scanning (already implemented)

Concurrent batch job processing (Phase 2)

Streaming responses for large outputs

Connection pooling for database

🔒 Phase 9: Security & Compliance (Weeks 17-18)

9.1 Security Hardening

Priority: 🔥 High

Effort: Medium (3-4 days)

Enhancements

API key encryption at rest

Secrets scanning in CI/CD

OWASP dependency check

Rate limiting per API key

9.2 Privacy Controls

Priority: 🟡 Medium

Effort: Small (2-3 days)

Features

Zero Data Retention (ZDR) routing option

Local-only mode (no API calls)

Data retention policy configuration

Audit logging

📦 Deliverable Timeline

Month 1: Foundation

├── Week 1-2: Planning System + Database

└── Week 3-4: Batch Jobs



Month 2: Architecture

├── Week 5-6: Refactoring + DI

└── Week 7-8: Cost Optimization



Month 3: Quality

├── Week 9-10: Observability

└── Week 11-12: Testing



Month 4: Polish

├── Week 13-14: Documentation + Examples

├── Week 15-16: Performance

└── Week 17-18: Security

🎯 Success Metrics

Key Performance Indicators (KPIs)

Autonomy: Agent can complete 80% of tasks without human intervention

Cost Efficiency: 50% reduction in API costs through smart routing

Developer Experience: 90%+ test coverage, comprehensive docs

Reliability: <1% failure rate on batch jobs

Performance: Sub-second response time for simple queries

🚧 Risks & Mitigations

RiskImpactMitigationAPI rate limits during batch jobsHighImplement exponential backoff + queue throttlingCost overruns from model usageHighHard limits + cost tracking + alertsBreaking changes to APIMediumVersion pinning + fallback chainsDatabase corruptionMediumRegular backups + transaction safety🔄 Iterative Approach

This roadmap follows an agile, iterative approach:



Build MVP of each feature first

Gather feedback from usage

Iterate and improve based on metrics

Ship incrementally - don't wait for perfection

📞 Next Steps

Review this roadmap - Adjust priorities based on your needs

Start with Phase 1 - Planning system provides immediate value

Track progress - Use GitHub Projects or similar

Celebrate wins - Each phase completion is a milestone!

Questions? Issues? Feedback?



Open an issue or discussion in the repository!
