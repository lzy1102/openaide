package services

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

type PostHookService struct {
	eventBus        *EventBus
	extractionSvc   KnowledgeExtractionService
	learningSvc     *LearningService
	reflectionSvc   *SelfReflectionService
	patternSvc      *PatternDetectorService
	skillEvolutionSvc *SkillEvolutionService
	capabilityGapSvc  *CapabilityGapService
	counters        map[string]int
	mu              sync.Mutex
}

func NewPostHookService(
	eventBus *EventBus,
	extractionSvc KnowledgeExtractionService,
	learningSvc *LearningService,
) *PostHookService {
	return &PostHookService{
		eventBus:      eventBus,
		extractionSvc: extractionSvc,
		learningSvc:   learningSvc,
		counters:      make(map[string]int),
	}
}

func (s *PostHookService) SetEvolutionServices(
	reflectionSvc *SelfReflectionService,
	patternSvc *PatternDetectorService,
	skillEvolutionSvc *SkillEvolutionService,
	capabilityGapSvc *CapabilityGapService,
) {
	s.reflectionSvc = reflectionSvc
	s.patternSvc = patternSvc
	s.skillEvolutionSvc = skillEvolutionSvc
	s.capabilityGapSvc = capabilityGapSvc
}

func (s *PostHookService) OnResponseComplete(ctx context.Context, dialogueID, userID, userQuery, assistantResponse string) {
	s.mu.Lock()
	s.counters[dialogueID]++
	count := s.counters[dialogueID]
	if len(s.counters) > 1000 {
		newCounters := make(map[string]int)
		for k, v := range s.counters {
			if v < 1000 {
				newCounters[k] = v
			}
		}
		s.counters = newCounters
	}
	s.mu.Unlock()

	if s.eventBus != nil && assistantResponse != "" {
		s.eventBus.Publish(ctx, models.EventTopicMessage, models.EventTypeMessageSent, "post_hook", map[string]interface{}{
			"dialogue_id": dialogueID,
			"content":     assistantResponse,
		})
	}

	go func() {
		extractCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		extracted, err := s.extractionSvc.ExtractFromDialogue(extractCtx, dialogueID)
		if err != nil {
			log.Printf("[PostHook] knowledge extraction failed for %s: %v", dialogueID, err)
			return
		}
		if len(extracted) > 0 && userID != "" {
			if saveErr := s.extractionSvc.AutoSave(extractCtx, extracted, userID); saveErr != nil {
				log.Printf("[PostHook] knowledge auto-save failed for %s: %v", dialogueID, saveErr)
			} else {
				log.Printf("[PostHook] knowledge extracted and saved: %d items from %s", len(extracted), dialogueID)
			}
		}
	}()

	if count%5 == 0 {
		go func() {
			optCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := s.learningSvc.GenerateOptimizationFromDialogue(optCtx, dialogueID); err != nil {
				log.Printf("[PostHook] optimization generation failed for %s: %v", dialogueID, err)
			}
		}()
	}

	if userID != "" && count%10 == 0 {
		go func() {
			patCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := s.learningSvc.AnalyzeInteractionPatterns(patCtx, userID); err != nil {
				log.Printf("[PostHook] pattern analysis failed for %s: %v", userID, err)
			}
		}()
	}

	if s.reflectionSvc != nil && userQuery != "" && assistantResponse != "" && count%3 == 0 {
		go func() {
			refCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if _, err := s.reflectionSvc.ReflectOnResponse(refCtx, dialogueID, userID, userQuery, assistantResponse); err != nil {
				log.Printf("[PostHook] self-reflection failed for %s: %v", dialogueID, err)
			}
		}()
	}

	if s.patternSvc != nil && userID != "" && count%20 == 0 {
		go func() {
			detCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			if _, err := s.patternSvc.DetectPatterns(detCtx, userID); err != nil {
				log.Printf("[PostHook] pattern detection failed for %s: %v", userID, err)
			}
		}()
	}
}

func (s *PostHookService) OnResponseCompleteLegacy(ctx context.Context, dialogueID, userID, content string) {
	s.OnResponseComplete(ctx, dialogueID, userID, "", content)
}

func (s *PostHookService) WrapStream(
	source <-chan llm.ChatStreamChunk,
	dialogueID, userID, userQuery string,
) <-chan llm.ChatStreamChunk {
	out := make(chan llm.ChatStreamChunk, 16)

	go func() {
		defer close(out)
		var fullContent strings.Builder
		for chunk := range source {
			out <- chunk
			if len(chunk.Choices) > 0 {
				fullContent.WriteString(chunk.Choices[0].Delta.Content)
			}
		}
		go s.OnResponseComplete(context.Background(), dialogueID, userID, userQuery, fullContent.String())
	}()

	return out
}
