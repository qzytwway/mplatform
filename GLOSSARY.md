# MPlatform

A console for operating Kubernetes clusters, including serving large language models on their GPUs.

## Language

### Inference Services

**Model**:
Offline model weights registered on the platform under a unique name; Callers request it by that name.
_Avoid_: Served model, registered model

**Engine**:
The inference server program, vLLM or SGLang, that loads a model onto GPUs and answers requests.
_Avoid_: Runtime, inference runtime, framework

**Inference Service**:
One Model running on one Engine in a cluster namespace, serving an OpenAI-compatible API. "Service" alone always means a Kubernetes Service.
_Avoid_: Service, model service, deployment (for the service itself)

**Replica**:
One complete copy of the Engine within an Inference Service that answers requests on its own, possibly spanning several nodes.
_Avoid_: Group

**Leader pod**:
The pod of a Replica spanning several nodes that receives its requests and coordinates the others.
_Avoid_: Leader role

**Worker pod**:
A pod of a Replica spanning several nodes other than its Leader pod.
_Avoid_: Worker role

**Deployment Mode**:
How an Inference Service lays out its Replicas: Single-node, Multi-node or PD disaggregation.
_Avoid_: Mode (alone), topology

**Multi-node**:
The Deployment Mode in which each Replica spans several nodes and prefill and decode run together. A PD disaggregation service whose Replicas span nodes is not Multi-node.

**PD disaggregation**:
The Deployment Mode that runs prefill and decode in separate Roles, which hand the KV cache from one to the other, behind a Router.
_Avoid_: PD mode, P/D split

**Role**:
A part of an Inference Service with its own Replica count: Prefill, Decode and Router under PD disaggregation, otherwise a single default Role.
_Avoid_: Component

### Gateway

**Gateway**:
A cluster's single entry point for Callers, which routes each request by its Model name to a published Inference Service.
_Avoid_: Model gateway, ingress

**Publish**:
To make the Gateway route requests for an Inference Service's Model to that Inference Service. A cluster has at most one published Inference Service per Model.
_Avoid_: Published model, expose

**Caller**:
A client of a cluster's Gateway with its own API key and the Models it may request.
_Avoid_: Consumer, client, tenant

**Rate limit**:
The most tokens a Caller may use per minute across all Models.
_Avoid_: Quota, TPM limit

**Quota**:
A Caller's total token budget, which rejects its requests once used up and never refills on its own.
_Avoid_: Rate limit, credit, balance
