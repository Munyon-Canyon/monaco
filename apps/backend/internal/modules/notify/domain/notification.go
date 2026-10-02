package domain

type NotificationState string

const (
	NotificationPending   NotificationState = "pending"
	NotificationDelivered NotificationState = "delivered"
	NotificationNoDevice  NotificationState = "no_device"
	NotificationBatched   NotificationState = "batched"
)
