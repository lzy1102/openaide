package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"openaide/backend/src/models"
)

type WorkflowStep string

const (
	WorkflowInterview WorkflowStep = "interview"
	WorkflowPlan      WorkflowStep = "plan"
	WorkflowExecute   WorkflowStep = "execute"
	WorkflowReview    WorkflowStep = "review"
	WorkflowComplete  WorkflowStep = "complete"
)

type WorkflowState struct {
	ID          string      `json:"id"`
	DialogueID  string      `json:"dialogue_id"`
	CurrentStep WorkflowStep `json:"current_step"`
	TaskDesc    string      `json:"task_desc"`
	InterviewQA []QAEntry   `json:"interview_qa,omitempty"`
	PlanContent string      `json:"plan_content,omitempty"`
	PlanApproved bool       `json:"plan_approved"`
	ExecResults []string    `json:"exec_results,omitempty"`
	ReviewNotes string      `json:"review_notes,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

type QAEntry struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type WorkflowService2 struct {
	mu          sync.RWMutex
	workflows   map[string]*WorkflowState
	eventBus    *EventBus
	dialogueSvc *DialogueService
}

func NewWorkflowService2(eventBus *EventBus, dialogueSvc *DialogueService) *WorkflowService2 {
	return &WorkflowService2{
		workflows:   make(map[string]*WorkflowState),
		eventBus:    eventBus,
		dialogueSvc: dialogueSvc,
	}
}

func (s *WorkflowService2) StartInterview(ctx context.Context, dialogueID, taskDesc string) (*WorkflowState, error) {
	wf := &WorkflowState{
		ID:          fmt.Sprintf("wf-%s", dialogueID),
		DialogueID:  dialogueID,
		CurrentStep: WorkflowInterview,
		TaskDesc:    taskDesc,
		InterviewQA: []QAEntry{},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	s.mu.Lock()
	s.workflows[dialogueID] = wf
	s.mu.Unlock()

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, models.EventTopicMessage, "workflow_started", "workflow", map[string]interface{}{
			"dialogue_id": dialogueID,
			"step":        string(WorkflowInterview),
			"task":        taskDesc,
		})
	}

	slog.Info("Workflow interview started", "component", "Workflow", "dialogue_id", dialogueID)
	return wf, nil
}

func (s *WorkflowService2) GetWorkflow(dialogueID string) *WorkflowState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.workflows[dialogueID]
}

func (s *WorkflowService2) AddInterviewQA(dialogueID, question, answer string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if wf, ok := s.workflows[dialogueID]; ok {
		wf.InterviewQA = append(wf.InterviewQA, QAEntry{Question: question, Answer: answer})
		wf.UpdatedAt = time.Now()
	}
}

func (s *WorkflowService2) TransitionToPlan(ctx context.Context, dialogueID string) (*WorkflowState, error) {
	s.mu.Lock()
	wf, ok := s.workflows[dialogueID]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("no active workflow for dialogue: %s", dialogueID)
	}
	wf.CurrentStep = WorkflowPlan
	wf.UpdatedAt = time.Now()
	s.mu.Unlock()

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, models.EventTopicMessage, "workflow_step", "workflow", map[string]interface{}{
			"dialogue_id": dialogueID,
			"step":        string(WorkflowPlan),
		})
	}

	return wf, nil
}

func (s *WorkflowService2) SetPlan(ctx context.Context, dialogueID, planContent string) (*WorkflowState, error) {
	s.mu.Lock()
	wf, ok := s.workflows[dialogueID]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("no active workflow for dialogue: %s", dialogueID)
	}
	wf.PlanContent = planContent
	wf.PlanApproved = false
	wf.UpdatedAt = time.Now()
	s.mu.Unlock()

	return wf, nil
}

func (s *WorkflowService2) ApprovePlan(ctx context.Context, dialogueID string) (*WorkflowState, error) {
	s.mu.Lock()
	wf, ok := s.workflows[dialogueID]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("no active workflow for dialogue: %s", dialogueID)
	}
	wf.PlanApproved = true
	wf.CurrentStep = WorkflowExecute
	wf.UpdatedAt = time.Now()
	s.mu.Unlock()

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, models.EventTopicMessage, "workflow_step", "workflow", map[string]interface{}{
			"dialogue_id":   dialogueID,
			"step":          string(WorkflowExecute),
			"plan_approved": true,
		})
	}

	return wf, nil
}

func (s *WorkflowService2) AddExecResult(dialogueID, result string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if wf, ok := s.workflows[dialogueID]; ok {
		wf.ExecResults = append(wf.ExecResults, result)
		wf.UpdatedAt = time.Now()
	}
}

func (s *WorkflowService2) TransitionToReview(ctx context.Context, dialogueID string) (*WorkflowState, error) {
	s.mu.Lock()
	wf, ok := s.workflows[dialogueID]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("no active workflow for dialogue: %s", dialogueID)
	}
	wf.CurrentStep = WorkflowReview
	wf.UpdatedAt = time.Now()
	s.mu.Unlock()

	return wf, nil
}

func (s *WorkflowService2) CompleteWorkflow(ctx context.Context, dialogueID, reviewNotes string) (*WorkflowState, error) {
	s.mu.Lock()
	wf, ok := s.workflows[dialogueID]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("no active workflow for dialogue: %s", dialogueID)
	}
	wf.CurrentStep = WorkflowComplete
	wf.ReviewNotes = reviewNotes
	wf.UpdatedAt = time.Now()
	s.mu.Unlock()

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, models.EventTopicMessage, "workflow_completed", "workflow", map[string]interface{}{
			"dialogue_id": dialogueID,
			"review":      reviewNotes,
		})
	}

	return wf, nil
}

func (s *WorkflowService2) BuildInterviewPrompt(dialogueID string) string {
	s.mu.RLock()
	wf, ok := s.workflows[dialogueID]
	s.mu.RUnlock()
	if !ok {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("[INTERVIEW MODE]\n")
	sb.WriteString("You are conducting a deep interview to clarify the task before planning.\n")
	sb.WriteString(fmt.Sprintf("Task: %s\n\n", wf.TaskDesc))
	if len(wf.InterviewQA) > 0 {
		sb.WriteString("Previous Q&A:\n")
		for i, qa := range wf.InterviewQA {
			sb.WriteString(fmt.Sprintf("%d. Q: %s\n   A: %s\n", i+1, qa.Question, qa.Answer))
		}
	}
	sb.WriteString("\nAsk clarifying questions about: scope, constraints, non-goals, success criteria, risks.\n")
	sb.WriteString("When you have enough information, say 'INTERVIEW_COMPLETE' and provide a summary.\n")
	return sb.String()
}

func (s *WorkflowService2) BuildPlanPrompt(dialogueID string) string {
	s.mu.RLock()
	wf, ok := s.workflows[dialogueID]
	s.mu.RUnlock()
	if !ok {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("[PLAN MODE]\n")
	sb.WriteString("Based on the interview, create a detailed implementation plan.\n\n")
	sb.WriteString(fmt.Sprintf("Task: %s\n\n", wf.TaskDesc))
	if len(wf.InterviewQA) > 0 {
		sb.WriteString("Interview findings:\n")
		for i, qa := range wf.InterviewQA {
			sb.WriteString(fmt.Sprintf("%d. %s → %s\n", i+1, qa.Question, qa.Answer))
		}
	}
	sb.WriteString("\nProvide:\n1. Architecture overview\n2. Step-by-step implementation plan\n3. Files to modify\n4. Risks and mitigations\n5. Success criteria\n")
	sb.WriteString("End with 'PLAN_READY' when done.\n")
	return sb.String()
}

func (s *WorkflowService2) BuildExecutePrompt(dialogueID string) string {
	s.mu.RLock()
	wf, ok := s.workflows[dialogueID]
	s.mu.RUnlock()
	if !ok {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("[EXECUTE MODE]\n")
	sb.WriteString("Execute the approved plan step by step.\n\n")
	sb.WriteString(fmt.Sprintf("Task: %s\n\n", wf.TaskDesc))
	if wf.PlanContent != "" {
		sb.WriteString(fmt.Sprintf("Approved Plan:\n%s\n\n", wf.PlanContent))
	}
	sb.WriteString("Implement each step carefully. After each step, verify the result before proceeding.\n")
	return sb.String()
}
