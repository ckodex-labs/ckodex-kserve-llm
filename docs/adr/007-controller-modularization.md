# ADR 007: Controller Modularization

## Status

Accepted

## Context

The `LLMInferenceServiceReconciler` had grown into a monolithic structure exceeding 1,516 lines. This made it difficult to:

1. Maintain and debug complex reconciliation logic.
2. Unit test individual components (e.g., Deployment construction vs. Status updates).
3. Extend the operator with new features (like GPU capacity monitoring) without further bloating the main loop.

## Decision

We have modularized the controller into a set of specialized sub-packages under `internal/controller/`:

### 1. `deployment` Package

- **`Builder`**: Responsible for constructing the Kubernetes `Deployment` and `Container` specifications.
- **`Hardware`**: Handles node-level hardware detection and optimization (vLLM arguments).
- **`GPUCapacity`**: Implements cluster-wide GPU resource monitoring and requirement checking.

### 2. `reconciler` Package

- **`ServiceReconciler`**: Manages the inference `Service` lifecycle.
- **`PDBReconciler`**: Manages the `PodDisruptionBudget`.
- **`Diff`**: Provides order-insensitive comparison utilities for `Containers`, `EnvVars`, and `Volumes`.

### 3. `status` Package

- **`Reconciler`**: Centralizes status updates, condition management, and "Optimization" tracking.

### 4. `cleanup` Package

- **`Reconciler`**: Handles resource finalization and safe deletion.

### 5. `api` Package

- Shared constants and logic for the controller layer (e.g., `FinalizerName`).

## Rationale

- **Separation of Concerns**: Each package has a single, well-defined responsibility.
- **Improved Testability**: We can now write isolated unit tests for `DeploymentBuilder` or `Diff` logic without spinning up a full manager or faking the entire `LLMInferenceService` state.
- **Developer Velocity**: New contributors can focus on a specific sub-package rather than navigating a monolithic file.
- **Atomic Reconciliation**: The main loop in `llminferenceservice_controller.go` is now a high-level orchestrator (~150 lines), making the overall flow much clearer.

## Consequences

- **Direct Refactoring**: Call sites in the main controller were updated to use these new components.
- **Initialization**: The `SetupWithManager` function now handles the instantiation of all sub-reconcilers.
- **Test Updates**: Any tests relying on internal controller methods may need to be updated to point to the new package structure.
