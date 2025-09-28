# **go-notification-service**

The **go-notification-service** is a dedicated, single-responsibility microservice for dispatching native push notifications to mobile platforms like iOS (APNS) and Android (FCM). It operates as a backend consumer, processing notification requests from a Google Cloud Pub/Sub topic.

### **Architecture**

The service is an asynchronous, event-driven pipeline built using components from the go-dataflow library. It contains no public API and is designed for high-throughput, resilient message processing.

**Core Workflow:**

1. **Consume**: The service listens to a configured Pub/Sub subscription for incoming NotificationRequest messages.
2. **Transform**: An initial pipeline step validates and unmarshals the raw message payload into a structured Go type.
3. **Process & Route**: The core processor groups device tokens by platform (ios, android).
4. **Dispatch**: The processor invokes the appropriate platform-specific Dispatcher (e.g., an APNS or FCM client) to send the notifications.

### **Key Features**

* **Single Responsibility**: Focused exclusively on handling native mobile push notifications.
* **Resilient by Design**: Built as a message processing pipeline that can be configured with Dead-Letter Queues (DLQs) to handle poison pill messages.
* **Extensible**: Easily supports new platforms by adding a new component that implements the notification.Dispatcher interface.
* **Horizontally Scalable**: As a stateless consumer, you can run multiple instances of the service to increase processing throughput.

### **Configuration**

The service is configured via config.yaml for non-secret values and environment variables for secrets (like APNS/FCM keys).

* project\_id: The Google Cloud project ID.
* subscription\_id: The Pub/Sub subscription to pull notification requests from.
* subscription\_dlq\_topic\_id: **(Required for Production)** The Dead-Letter Topic for the subscription.
* num\_workers: The number of concurrent workers to process messages.

### **Running the Service**

1. **Configure config.yaml**.
2. **Set Environment Variables** (for real dispatchers):  
   \# Example for future implementation  
   export APNS\_KEY\_ID="YOUR\_KEY\_ID"  
   export FCM\_SERVER\_KEY="YOUR\_SERVER\_KEY"

3. **Run the Service**:  
   go run ./cmd/notificationservice  
