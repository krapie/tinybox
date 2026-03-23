package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	api "github.com/krapi0314/tinybox/tinykube/api/v1"
)

const defaultServer = "http://localhost:8080"
const defaultNamespace = "default"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}
	out, err := runCmd(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Print(out)
}

// runCmd dispatches the command and returns output as a string (testable without os.Exit).
func runCmd(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("no command given")
	}
	switch args[0] {
	case "apply":
		return cmdApply(args[1:])
	case "get":
		return cmdGet(args[1:])
	case "delete":
		return cmdDelete(args[1:])
	case "status":
		return cmdStatus(args[1:])
	default:
		return "", fmt.Errorf("unknown command %q — valid commands: apply, get, delete, status", args[0])
	}
}

// --- apply ---

func cmdApply(args []string) (string, error) {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	server := fs.String("server", defaultServer, "tinykube API server address")
	ns := fs.String("namespace", defaultNamespace, "namespace")
	file := fs.String("f", "", "path to a YAML manifest file")
	name := fs.String("name", "", "deployment name")
	image := fs.String("image", "", "container image")
	replicas := fs.Int("replicas", 1, "number of replicas")
	port := fs.Int("port", 80, "container port")
	maxSurge := fs.Int("max-surge", 1, "max surge during rolling update")
	maxUnavailable := fs.Int("max-unavailable", 1, "max unavailable during rolling update")

	if err := fs.Parse(args); err != nil {
		return "", err
	}

	// -f and --name are mutually exclusive
	if *file != "" && *name != "" {
		return "", fmt.Errorf("-f and --name are mutually exclusive — use one or the other")
	}

	var dep api.Deployment

	if *file != "" {
		// Manifest mode: parse YAML file.
		parsedDep, parsedSvc, err := parseManifestFile(*file)
		if err != nil {
			return "", err
		}
		if parsedSvc != nil {
			// Service manifest — delegate to service apply.
			if *ns != defaultNamespace {
				parsedSvc.Namespace = *ns
			}
			return applyService(*server, parsedSvc)
		}
		dep = *parsedDep
		// --server and --namespace flags still apply as overrides.
		if *ns != defaultNamespace {
			dep.Namespace = *ns
		}
	} else {
		// Flag mode: build from explicit flags.
		if *name == "" || *image == "" {
			return "", fmt.Errorf("--name and --image are required (or use -f <manifest.yaml>)")
		}
		dep = api.Deployment{
			Name:      *name,
			Namespace: *ns,
			Spec: api.DeploymentSpec{
				Replicas: *replicas,
				Selector: map[string]string{"app": *name},
				Template: api.PodTemplate{
					Labels: map[string]string{"app": *name},
					Spec:   api.PodSpec{Image: *image, Port: *port},
				},
				Strategy: api.RollingUpdateStrategy{
					MaxSurge:       *maxSurge,
					MaxUnavailable: *maxUnavailable,
				},
			},
		}
	}

	return applyDeployment(*server, &dep)
}

// applyDeployment creates or updates a deployment via the API server.
func applyDeployment(server string, dep *api.Deployment) (string, error) {
	body, _ := json.Marshal(dep)
	base := fmt.Sprintf("%s/apis/apps/v1/namespaces/%s/deployments", server, dep.Namespace)

	// Check if it already exists.
	checkResp, err := http.Get(base + "/" + dep.Name)
	if err != nil {
		return "", fmt.Errorf("connecting to server: %w", err)
	}
	_ = checkResp.Body.Close()

	var result api.Deployment
	if checkResp.StatusCode == http.StatusOK {
		r, err := doRequest(http.MethodPut, base+"/"+dep.Name, bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		if err := json.Unmarshal(r, &result); err != nil {
			return "", fmt.Errorf("parse response: %w", err)
		}
		return fmt.Sprintf("deployment/%s updated\n", result.Name), nil
	}

	r, err := doRequest(http.MethodPost, base, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(r, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	return fmt.Sprintf("deployment/%s created\n", result.Name), nil
}

// --- get ---

func cmdGet(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("specify resource type: deployments or pods")
	}
	resource := args[0]
	rest := args[1:]

	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	server := fs.String("server", defaultServer, "tinykube API server address")
	ns := fs.String("namespace", defaultNamespace, "namespace")
	if err := fs.Parse(rest); err != nil {
		return "", err
	}

	switch strings.ToLower(resource) {
	case "deployments", "deployment":
		return getDeployments(*server, *ns)
	case "pods", "pod":
		return getPods(*server, *ns)
	case "services", "service":
		return getServices(*server, *ns)
	default:
		return "", fmt.Errorf("unknown resource %q — use deployments, pods, or services", resource)
	}
}

func getDeployments(server, ns string) (string, error) {
	url := fmt.Sprintf("%s/apis/apps/v1/namespaces/%s/deployments", server, ns)
	body, err := doRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	var deps []api.Deployment
	if err := json.Unmarshal(body, &deps); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	var sb strings.Builder
	w := tabwriter.NewWriter(&sb, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tREPLICAS\tREADY\tIMAGE")
	for _, d := range deps {
		fmt.Fprintf(w, "%s\t%d\t%d\t%s\n",
			d.Name,
			d.Spec.Replicas,
			d.Status.ReadyReplicas,
			d.Spec.Template.Spec.Image,
		)
	}
	_ = w.Flush()
	return sb.String(), nil
}

func getPods(server, ns string) (string, error) {
	url := fmt.Sprintf("%s/apis/v1/namespaces/%s/pods", server, ns)
	body, err := doRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	var pods []api.Pod
	if err := json.Unmarshal(body, &pods); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	var sb strings.Builder
	w := tabwriter.NewWriter(&sb, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tIP\tIMAGE")
	for _, p := range pods {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			p.Name,
			p.Status,
			p.PodIP,
			p.Spec.Image,
		)
	}
	_ = w.Flush()
	return sb.String(), nil
}

// --- delete ---

func cmdDelete(args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("usage: delete deployment <name>")
	}
	resource := args[0]
	rest := args[1:]

	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	server := fs.String("server", defaultServer, "tinykube API server address")
	ns := fs.String("namespace", defaultNamespace, "namespace")

	// name is the first positional arg before flags
	name := rest[0]
	if err := fs.Parse(rest[1:]); err != nil {
		return "", err
	}

	switch strings.ToLower(resource) {
	case "deployment":
		url := fmt.Sprintf("%s/apis/apps/v1/namespaces/%s/deployments/%s", *server, *ns, name)
		req, err := http.NewRequest(http.MethodDelete, url, nil)
		if err != nil {
			return "", err
		}
		httpResp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("connecting to server: %w", err)
		}
		_ = httpResp.Body.Close()
		if httpResp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("deployment %q not found", name)
		}
		return fmt.Sprintf("deployment/%s deleted\n", name), nil
	case "service":
		url := fmt.Sprintf("%s/apis/v1/namespaces/%s/services/%s", *server, *ns, name)
		req, err := http.NewRequest(http.MethodDelete, url, nil)
		if err != nil {
			return "", err
		}
		httpResp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("connecting to server: %w", err)
		}
		_ = httpResp.Body.Close()
		if httpResp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("service %q not found", name)
		}
		return fmt.Sprintf("service/%s deleted\n", name), nil
	default:
		return "", fmt.Errorf("unknown resource %q — use deployment or service", resource)
	}
}

// --- status ---

func cmdStatus(args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("usage: status deployment <name>")
	}
	resource := args[0]
	rest := args[1:]

	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	server := fs.String("server", defaultServer, "tinykube API server address")
	ns := fs.String("namespace", defaultNamespace, "namespace")

	name := rest[0]
	if err := fs.Parse(rest[1:]); err != nil {
		return "", err
	}

	switch strings.ToLower(resource) {
	case "deployment":
		url := fmt.Sprintf("%s/apis/apps/v1/namespaces/%s/deployments/%s/status", *server, *ns, name)
		body, err := doRequest(http.MethodGet, url, nil)
		if err != nil {
			return "", err
		}
		var status api.DeploymentStatus
		if err := json.Unmarshal(body, &status); err != nil {
			return "", fmt.Errorf("parse response: %w", err)
		}
		var sb strings.Builder
		w := tabwriter.NewWriter(&sb, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "FIELD\tVALUE")
		fmt.Fprintf(w, "Replicas\t%d\n", status.Replicas)
		fmt.Fprintf(w, "ReadyReplicas\t%d\n", status.ReadyReplicas)
		fmt.Fprintf(w, "AvailableReplicas\t%d\n", status.AvailableReplicas)
		fmt.Fprintf(w, "UpdatedReplicas\t%d\n", status.UpdatedReplicas)
		_ = w.Flush()
		return sb.String(), nil
	default:
		return "", fmt.Errorf("unknown resource %q — use deployment", resource)
	}
}

// --- helpers ---

func doRequest(method, url string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connecting to server: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}

// applyService creates or updates a Service via the API server.
func applyService(server string, svc *api.Service) (string, error) {
	body, _ := json.Marshal(svc)
	base := fmt.Sprintf("%s/apis/v1/namespaces/%s/services", server, svc.Namespace)

	checkResp, err := http.Get(base + "/" + svc.Name)
	if err != nil {
		return "", fmt.Errorf("connecting to server: %w", err)
	}
	_ = checkResp.Body.Close()

	var result api.Service
	if checkResp.StatusCode == http.StatusOK {
		r, err := doRequest(http.MethodPut, base+"/"+svc.Name, bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		if err := json.Unmarshal(r, &result); err != nil {
			return "", fmt.Errorf("parse response: %w", err)
		}
		return fmt.Sprintf("service/%s updated\n", result.Name), nil
	}

	r, err := doRequest(http.MethodPost, base, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(r, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	return fmt.Sprintf("service/%s created\n", result.Name), nil
}

func getServices(server, ns string) (string, error) {
	url := fmt.Sprintf("%s/apis/v1/namespaces/%s/services", server, ns)
	body, err := doRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	var svcs []api.Service
	if err := json.Unmarshal(body, &svcs); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	var sb strings.Builder
	w := tabwriter.NewWriter(&sb, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tNAMESPACE\tPORT\tSELECTOR")
	for _, svc := range svcs {
		selector := ""
		for k, v := range svc.Spec.Selector {
			if selector != "" {
				selector += ","
			}
			selector += k + "=" + v
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", svc.Name, svc.Namespace, svc.Spec.Port, selector)
	}
	_ = w.Flush()
	return sb.String(), nil
}

func printUsage() {
	fmt.Println(`tkctl — tinykube CLI

Usage:
  tkctl apply   --name <name> --image <image> [--replicas <n>] [--port <p>]
                [--namespace <ns>] [--max-surge <n>] [--max-unavailable <n>]
                [--server <addr>]
  tkctl apply   -f <manifest.yaml> [--namespace <ns>] [--server <addr>]
  tkctl get     deployments|pods|services [--namespace <ns>] [--server <addr>]
  tkctl delete  deployment|service <name> [--namespace <ns>] [--server <addr>]
  tkctl status  deployment <name> [--namespace <ns>] [--server <addr>]

Flags default to --namespace=default --server=http://localhost:8080`)
}
