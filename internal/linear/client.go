package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/zeefan1555/symphony-go/internal/types"
)

const pollQuery = `
query SymphonyGoPoll($projectSlug: String!, $stateNames: [String!]!, $first: Int!, $after: String) {
  issues(filter: {project: {slugId: {eq: $projectSlug}}, state: {name: {in: $stateNames}}}, first: $first, after: $after) {
    nodes {
      id
      identifier
      title
      description
      priority
      state { name }
      branchName
      url
      labels { nodes { name } }
      inverseRelations { nodes { type issue { id identifier state { name } } } }
      createdAt
      updatedAt
    }
    pageInfo { hasNextPage endCursor }
  }
}`

const issueByIDQuery = `
query SymphonyGoIssueById($id: String!) {
  issue(id: $id) {
    id
    identifier
    title
    description
    priority
    state { name }
    branchName
    url
    labels { nodes { name } }
    inverseRelations { nodes { type issue { id identifier state { name } } } }
    createdAt
    updatedAt
  }
}`

const issueStatesByIDsQuery = `
query SymphonyGoIssueStatesByIDs($ids: [ID!], $first: Int!) {
  issues(filter: {id: {in: $ids}}, first: $first) {
    nodes {
      id
      identifier
      state { name }
    }
  }
}`

const stateLookupQuery = `
query SymphonyGoResolveStateId($issueId: String!, $stateName: String!) {
  issue(id: $issueId) {
    team {
      states(filter: {name: {eq: $stateName}}, first: 1) {
        nodes { id }
      }
    }
  }
}`

const updateStateMutation = `
mutation SymphonyGoUpdateIssueState($issueId: String!, $stateId: String!) {
  issueUpdate(id: $issueId, input: {stateId: $stateId}) { success }
}`

const createCommentMutation = `
mutation SymphonyGoCreateComment($issueId: String!, $body: String!) {
  commentCreate(input: {issueId: $issueId, body: $body}) { success comment { id } }
}`

const commentsQuery = `
query SymphonyGoIssueComments($issueId: String!) {
  issue(id: $issueId) {
    comments(first: 50) {
      nodes { id body }
    }
  }
}`

const updateCommentMutation = `
mutation SymphonyGoUpdateComment($commentId: String!, $body: String!) {
  commentUpdate(id: $commentId, input: {body: $body}) { success }
}`

type Client struct {
	Endpoint    string
	APIKey      string
	ProjectSlug string
	HTTPClient  *http.Client
}

func New(cfg types.TrackerConfig) (*Client, error) {
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("LINEAR_API_KEY")
	}
	if apiKey == "" {
		return nil, errors.New("missing Linear API key")
	}
	if cfg.ProjectSlug == "" {
		return nil, errors.New("missing Linear project slug")
	}
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = "https://api.linear.app/graphql"
	}
	return &Client{
		Endpoint:    endpoint,
		APIKey:      apiKey,
		ProjectSlug: cfg.ProjectSlug,
		HTTPClient:  &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (c *Client) FetchActiveIssues(ctx context.Context, states []string) ([]types.Issue, error) {
	return c.FetchIssuesByStates(ctx, states)
}

func (c *Client) FetchIssuesByStates(ctx context.Context, states []string) ([]types.Issue, error) {
	if len(states) == 0 {
		return []types.Issue{}, nil
	}
	var issues []types.Issue
	var after *string
	for {
		var body struct {
			Data struct {
				Issues rawIssueConnection `json:"issues"`
			} `json:"data"`
		}
		if err := c.GraphQL(ctx, pollQuery, map[string]any{
			"projectSlug": c.ProjectSlug,
			"stateNames":  states,
			"first":       50,
			"after":       after,
		}, &body); err != nil {
			return nil, err
		}
		issues = append(issues, normalizeIssues(body.Data.Issues.Nodes)...)
		if !body.Data.Issues.PageInfo.HasNextPage {
			return issues, nil
		}
		if body.Data.Issues.PageInfo.EndCursor == nil {
			return nil, errors.New("linear_missing_end_cursor")
		}
		after = body.Data.Issues.PageInfo.EndCursor
	}
}

func (c *Client) FetchIssue(ctx context.Context, id string) (types.Issue, error) {
	var body struct {
		Data struct {
			Issue rawIssue `json:"issue"`
		} `json:"data"`
	}
	if err := c.GraphQL(ctx, issueByIDQuery, map[string]any{"id": id}, &body); err != nil {
		return types.Issue{}, err
	}
	return normalizeIssue(body.Data.Issue), nil
}

func (c *Client) FetchIssueStatesByIDs(ctx context.Context, ids []string) ([]types.Issue, error) {
	if len(ids) == 0 {
		return []types.Issue{}, nil
	}
	var body struct {
		Data struct {
			Issues struct {
				Nodes []rawIssue `json:"nodes"`
			} `json:"issues"`
		} `json:"data"`
	}
	if err := c.GraphQL(ctx, issueStatesByIDsQuery, map[string]any{
		"ids":   ids,
		"first": len(ids),
	}, &body); err != nil {
		return nil, err
	}
	return normalizeIssues(body.Data.Issues.Nodes), nil
}

func (c *Client) UpdateIssueState(ctx context.Context, issueID, stateName string) error {
	stateID, err := c.resolveStateID(ctx, issueID, stateName)
	if err != nil {
		return err
	}
	var body struct {
		Data struct {
			IssueUpdate struct {
				Success bool `json:"success"`
			} `json:"issueUpdate"`
		} `json:"data"`
	}
	if err := c.GraphQL(ctx, updateStateMutation, map[string]any{"issueId": issueID, "stateId": stateID}, &body); err != nil {
		return err
	}
	if !body.Data.IssueUpdate.Success {
		return errors.New("Linear issueUpdate returned success=false")
	}
	return nil
}

func (c *Client) CreateComment(ctx context.Context, issueID, bodyText string) error {
	var body struct {
		Data struct {
			CommentCreate struct {
				Success bool `json:"success"`
			} `json:"commentCreate"`
		} `json:"data"`
	}
	if err := c.GraphQL(ctx, createCommentMutation, map[string]any{"issueId": issueID, "body": bodyText}, &body); err != nil {
		return err
	}
	if !body.Data.CommentCreate.Success {
		return errors.New("Linear commentCreate returned success=false")
	}
	return nil
}

func (c *Client) UpsertWorkpad(ctx context.Context, issueID, bodyText string) error {
	commentID, err := c.findWorkpadComment(ctx, issueID)
	if err != nil {
		return err
	}
	if commentID == "" {
		return c.CreateComment(ctx, issueID, bodyText)
	}
	var body struct {
		Data struct {
			CommentUpdate struct {
				Success bool `json:"success"`
			} `json:"commentUpdate"`
		} `json:"data"`
	}
	if err := c.GraphQL(ctx, updateCommentMutation, map[string]any{"commentId": commentID, "body": bodyText}, &body); err != nil {
		return err
	}
	if !body.Data.CommentUpdate.Success {
		return errors.New("Linear commentUpdate returned success=false")
	}
	return nil
}

func (c *Client) findWorkpadComment(ctx context.Context, issueID string) (string, error) {
	var body struct {
		Data struct {
			Issue struct {
				Comments struct {
					Nodes []struct {
						ID   string `json:"id"`
						Body string `json:"body"`
					} `json:"nodes"`
				} `json:"comments"`
			} `json:"issue"`
		} `json:"data"`
	}
	if err := c.GraphQL(ctx, commentsQuery, map[string]any{"issueId": issueID}, &body); err != nil {
		return "", err
	}
	for _, comment := range body.Data.Issue.Comments.Nodes {
		if strings.Contains(comment.Body, "## Codex Workpad") {
			return comment.ID, nil
		}
	}
	return "", nil
}

func (c *Client) GraphQL(ctx context.Context, query string, variables map[string]any, out any) error {
	payload := map[string]any{"query": query, "variables": variables}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.APIKey)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Linear GraphQL status %d: %s", resp.StatusCode, string(respBody))
	}
	var envelope struct {
		Errors []any `json:"errors"`
	}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return err
	}
	if len(envelope.Errors) > 0 {
		return fmt.Errorf("Linear GraphQL errors: %v", envelope.Errors)
	}
	return json.Unmarshal(respBody, out)
}

func (c *Client) resolveStateID(ctx context.Context, issueID, stateName string) (string, error) {
	var body struct {
		Data struct {
			Issue struct {
				Team struct {
					States struct {
						Nodes []struct {
							ID string `json:"id"`
						} `json:"nodes"`
					} `json:"states"`
				} `json:"team"`
			} `json:"issue"`
		} `json:"data"`
	}
	if err := c.GraphQL(ctx, stateLookupQuery, map[string]any{"issueId": issueID, "stateName": stateName}, &body); err != nil {
		return "", err
	}
	if len(body.Data.Issue.Team.States.Nodes) == 0 || body.Data.Issue.Team.States.Nodes[0].ID == "" {
		return "", fmt.Errorf("Linear state %q not found", stateName)
	}
	return body.Data.Issue.Team.States.Nodes[0].ID, nil
}

type rawIssue struct {
	ID          string `json:"id"`
	Identifier  string `json:"identifier"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    *int   `json:"priority"`
	State       struct {
		Name string `json:"name"`
	} `json:"state"`
	BranchName string `json:"branchName"`
	URL        string `json:"url"`
	Labels     struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`
	InverseRelations struct {
		Nodes []struct {
			Type  string `json:"type"`
			Issue *struct {
				ID         string `json:"id"`
				Identifier string `json:"identifier"`
				State      struct {
					Name string `json:"name"`
				} `json:"state"`
			} `json:"issue"`
		} `json:"nodes"`
	} `json:"inverseRelations"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type rawIssueConnection struct {
	Nodes    []rawIssue `json:"nodes"`
	PageInfo struct {
		HasNextPage bool    `json:"hasNextPage"`
		EndCursor   *string `json:"endCursor"`
	} `json:"pageInfo"`
}

func normalizeIssues(raw []rawIssue) []types.Issue {
	issues := make([]types.Issue, 0, len(raw))
	for _, item := range raw {
		issues = append(issues, normalizeIssue(item))
	}
	return issues
}

func normalizeIssue(raw rawIssue) types.Issue {
	labels := make([]string, 0, len(raw.Labels.Nodes))
	for _, label := range raw.Labels.Nodes {
		if label.Name != "" {
			labels = append(labels, strings.ToLower(label.Name))
		}
	}
	blockers := make([]types.BlockerRef, 0, len(raw.InverseRelations.Nodes))
	for _, relation := range raw.InverseRelations.Nodes {
		if relation.Type != "blocks" || relation.Issue == nil {
			continue
		}
		blockers = append(blockers, types.BlockerRef{
			ID:         relation.Issue.ID,
			Identifier: relation.Issue.Identifier,
			State:      relation.Issue.State.Name,
		})
	}
	return types.Issue{
		ID:          raw.ID,
		Identifier:  raw.Identifier,
		Title:       raw.Title,
		Description: raw.Description,
		Priority:    raw.Priority,
		State:       raw.State.Name,
		BranchName:  raw.BranchName,
		URL:         raw.URL,
		Labels:      labels,
		BlockedBy:   blockers,
		CreatedAt:   parseTime(raw.CreatedAt),
		UpdatedAt:   parseTime(raw.UpdatedAt),
	}
}

func parseTime(value string) *time.Time {
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return &parsed
}
