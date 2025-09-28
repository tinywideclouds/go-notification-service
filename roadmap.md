# **Roadmap to Production**

This document outlines the engineering tasks required to make the go-notification-service fully production-ready. The core pipeline is functional with placeholder dispatchers; these steps focus on hardening the service and implementing the production clients.

### **Phase 1: Production Hardening (Highest Priority)**

* **1\. Standardize Service Lifecycle**
    * **Task**: Refactor the service to use the go-microservice-base library. The service.Wrapper will embed microservice.BaseServer to gain standardized health/readiness probes, metrics, and graceful shutdown logic.
    * **Purpose**: To align with other microservices, reduce boilerplate code, and improve operational readiness for environments like Kubernetes.
* **2\. Configure Dead-Letter Queues (DLQ)**
    * **Task**: Update the main entrypoint to programmatically create the Pub/Sub subscription with a DeadLetterPolicy configured. Add the DLQ topic name to config.yaml.
    * **Purpose**: This is a **critical resilience pattern** to prevent "poison pill" messages from blocking the entire notification pipeline.
* **3\. Enhance Test Coverage**
    * **Task**: Add an integration test case where a mock dispatcher returns an error, and verify that the message is redelivered by Pub/Sub for a retry.
    * **Purpose**: To ensure the pipeline's error handling and retry mechanism works as expected.

### **Phase 2: Production Dispatcher Implementation**

* **4\. Implement Real Dispatchers**
    * **Task**: Replace the current LoggingDispatcher placeholders with real clients for APNS and FCM in internal/platform/. This will involve securely loading credentials from the environment and handling vendor-specific API responses.
    * **Purpose**: To enable the service to send actual push notifications.
* **5\. Implement the Feedback Loop**
    * **Task**: Enhance the real dispatchers to detect responses from Apple/Google that indicate a device token is expired or invalid. When detected, publish a structured event (e.g., TokenInvalidated) to a new "push-feedback" Pub/Sub topic.
    * **Purpose**: To close the loop and allow other services (like the go-routing-service) to clean up stale device tokens from their database.

### **Phase 3: Observability**

* **6\. Integrate Distributed Tracing**
    * **Task**: Integrate OpenTelemetry to trace a notification's journey from the moment it's consumed from Pub/Sub to the moment it's dispatched.
    * **Purpose**: To provide essential visibility for debugging latency and failures.