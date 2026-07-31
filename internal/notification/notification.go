package notification

import (
	"gorm.io/gorm"

	"aeo_geo_seo_agent/internal/database"
)

type Manager struct {
	db *gorm.DB
}

func NewManager(db *gorm.DB) *Manager {
	return &Manager{db: db}
}

func (m *Manager) GetUserNotifications(userID uint, role string) ([]database.Notification, error) {
	var notifs []database.Notification
	err := m.db.Where("user_id = ? OR (user_role = ? AND user_id = 0)", userID, role).
		Order("created_at DESC").
		Limit(30).
		Find(&notifs).Error
	return notifs, err
}

func (m *Manager) MarkAsRead(notifID uint) error {
	return m.db.Model(&database.Notification{}).Where("id = ?", notifID).Update("read", true).Error
}

func (m *Manager) CreateNotification(userID uint, role, title, message, notifType string) error {
	n := database.Notification{
		UserID:    userID,
		UserRole:  role,
		Title:     title,
		Message:   message,
		Type:      notifType,
		Read:      false,
	}
	return m.db.Create(&n).Error
}
