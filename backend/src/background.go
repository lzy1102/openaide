package main

import (
	"context"
	"log"
	"time"
)

// BackgroundTasks 管理所有后台任务
type BackgroundTasks struct {
	app *Application
}

// NewBackgroundTasks 创建后台任务管理器
func NewBackgroundTasks(app *Application) *BackgroundTasks {
	return &BackgroundTasks{app: app}
}

// Start 启动所有后台任务
func (b *BackgroundTasks) Start() {
	// 启动自动记忆提取服务
	b.startMemoryExtraction()

	// 启动技能进化定时任务
	b.startSkillEvolution()

	// 启动技能发现服务
	b.startSkillDiscovery()

	// 启动模式检测器
	b.startPatternDetection()

	// 启动用户反馈收集器
	b.startUserFeedbackCollection()

	log.Printf("[Self-Evolution] Skill discovery, pattern detection, and feedback collection enabled")
}

// startMemoryExtraction 启动自动记忆提取
func (b *BackgroundTasks) startMemoryExtraction() {
	go func() {
		time.Sleep(5 * time.Second)
		if err := b.app.MemoryExtractionService.BatchExtractPendingDialogues("", 5); err != nil {
			log.Printf("[MemoryExtract] batch extraction failed: %v", err)
		}
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if err := b.app.MemoryExtractionService.BatchExtractPendingDialogues("", 10); err != nil {
				log.Printf("[MemoryExtract] periodic extraction failed: %v", err)
			}
		}
	}()
}

// startSkillEvolution 启动技能进化定时任务
func (b *BackgroundTasks) startSkillEvolution() {
	go b.app.SkillEvolutionService.RunPeriodicEvolution(context.Background())
}

// startSkillDiscovery 启动技能发现服务
func (b *BackgroundTasks) startSkillDiscovery() {
	go func() {
		time.Sleep(30 * time.Second)
		b.app.SkillDiscoveryService.RunPeriodicDiscovery(context.Background())
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			b.app.SkillDiscoveryService.RunPeriodicDiscovery(context.Background())
		}
	}()
}

// startPatternDetection 启动模式检测器
func (b *BackgroundTasks) startPatternDetection() {
	go func() {
		time.Sleep(60 * time.Second)
		b.app.PatternDetector.RunPeriodicPatternDetection(context.Background())
		ticker := time.NewTicker(12 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			b.app.PatternDetector.RunPeriodicPatternDetection(context.Background())
		}
	}()
}

// startUserFeedbackCollection 启动用户反馈收集器
func (b *BackgroundTasks) startUserFeedbackCollection() {
	go func() {
		time.Sleep(90 * time.Second)
		b.app.UserFeedbackCollector.RunPeriodicFeedbackAnalysis(context.Background())
		ticker := time.NewTicker(4 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			b.app.UserFeedbackCollector.RunPeriodicFeedbackAnalysis(context.Background())
		}
	}()
}
