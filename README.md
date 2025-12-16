# Go Notification Service

A dedicated microservice for handling multi-platform push notifications. This service acts as the "Muscle" in the backend architecture, responsible for the physical delivery of messages to user devices via Firebase Cloud Messaging (FCM).

## 🏗 Architecture

The Notification Service operates as an event-driven consumer in the platform's routing pipeline.



1.  **Event Driven:** Listens to Pub/Sub topics for "Notify User" commands.
2.  **Decoupled:** It is the *sole owner* of device token storage. The Routing Service sends a user ID, and this service resolves it to actual device tokens (FCM/APNS).
3.  **Platform Agnostic:** Supports dispatching to Web (PWA), Android, and iOS via a unified FCM adapter.

## 🚀 Features

* **Token Management:** REST API (`PUT /tokens`) for devices to register their FCM tokens.
* **Smart Dispatch:** Looks up all active devices for a user and multicasts notifications.
* **Scalable:** Deploys as a stateless container on Cloud Run; scales to zero when idle.
* **Secure:** Uses Google Secret Manager for sensitive service account credentials.

## 🔌 API Endpoints

### Register Device Token
Used by the frontend (Angular PWA/Mobile) to link a device to the current user.

* **URL:** `PUT /tokens`
* **Auth:** Requires valid JWT (Bearer Token)
* **Body:**
    ```json
    {
      "token": "fcm-registration-token-from-browser",
      "platform": "web"
    }
    ```

## 🛠 Setup & Configuration

### Prerequisites
* **Firebase Project:** A project set up in the Firebase Console to handle messaging.
* **Service Account:** A `service-account.json` key file from Firebase with "Firebase Admin" permissions.

### Environment Variables

| Variable | Description | Example |
| :--- | :--- | :--- |
| `GCP_PROJECT_ID` | Google Cloud Project ID | `my-project-id` |
| `IDENTITY_SERVICE_URL` | URL of the Identity Service (for JWT validation) | `https://identity-service.run.app` |
| `GOOGLE_APPLICATION_CREDENTIALS` | Path to the Firebase Service Account key | `/secrets/service-account.json` |
| `LOG_LEVEL` | Logging verbosity | `info` / `debug` |

### Local Development

1.  **Place Key:** Drop your `service-account.json` in the project root (ensure it is `.gitignored`).
2.  **Export Creds:**
    ```bash
    export GOOGLE_APPLICATION_CREDENTIALS="./service-account.json"
    ```
3.  **Run:**
    ```bash
    go run cmd/notificationservice/runnotificationservice.go
    ```

## 📦 Deployment (Cloud Run)

This service is designed for Source-based deployment to Google Cloud Run.

```bash
gcloud run deploy notification-service \
  --source . \
  --region us-central1 \
  --allow-unauthenticated \
  --set-env-vars="GCP_PROJECT_ID=[YOUR_PROJECT_ID]" \
  --set-env-vars="IDENTITY_SERVICE_URL=[YOUR_ID_URL]" \
  --set-env-vars="GOOGLE_APPLICATION_CREDENTIALS=/secrets/service-account.json" \
  --set-secrets="/secrets/service-account.json=fcm-service-account:latest"
  ```