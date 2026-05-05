package model

import (
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm/clause"
)

type TaskSubmitRepair struct {
	CreatedAt   int64           `json:"created_at" gorm:"index"`
	UpdatedAt   int64           `json:"updated_at"`
	TaskID      string          `json:"task_id" gorm:"primaryKey;type:varchar(191)"`
	PrivateData TaskPrivateData `json:"private_data" gorm:"type:json"`
	Data        json.RawMessage `json:"data" gorm:"type:json"`
	ExpiresAt   int64           `json:"expires_at" gorm:"index"`
}

func UpsertTaskSubmitRepair(task *Task, expiresAt int64) error {
	if task == nil || DB == nil {
		return nil
	}
	taskID := strings.TrimSpace(task.TaskID)
	if taskID == "" {
		return nil
	}
	now := time.Now().Unix()
	repair := &TaskSubmitRepair{
		CreatedAt:   now,
		UpdatedAt:   now,
		TaskID:      taskID,
		PrivateData: task.PrivateData,
		Data:        append([]byte(nil), task.Data...),
		ExpiresAt:   expiresAt,
	}
	return DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "task_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"updated_at", "private_data", "data", "expires_at"}),
	}).Create(repair).Error
}

func GetTaskSubmitRepair(taskID string) (*TaskSubmitRepair, bool, error) {
	if DB == nil {
		return nil, false, nil
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, false, nil
	}
	var repair TaskSubmitRepair
	err := DB.Where("task_id = ?", taskID).First(&repair).Error
	exist, err := RecordExist(err)
	if err != nil || !exist {
		return nil, exist, err
	}
	if repair.ExpiresAt > 0 && repair.ExpiresAt < time.Now().Unix() {
		_ = DeleteTaskSubmitRepair(taskID)
		return nil, false, nil
	}
	return &repair, true, nil
}

func DeleteTaskSubmitRepair(taskID string) error {
	if DB == nil {
		return nil
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil
	}
	return DB.Where("task_id = ?", taskID).Delete(&TaskSubmitRepair{}).Error
}
