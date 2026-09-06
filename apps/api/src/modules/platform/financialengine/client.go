package financialengine

import (
	"context"
	"errors"
	"net/netip"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	financialenginepb "ledgermeadow/src/modules/platform/financialengine/pb"
)

const maxMessageBytes = 4 << 20

type Client struct {
	connection *grpc.ClientConn
	client     financialenginepb.FinancialEngineClient
}

func NewClient(address string) (*Client, error) {
	if address == "" {
		return nil, errors.New("financial engine address is required")
	}
	endpoint, err := netip.ParseAddrPort(address)
	if err != nil || !endpoint.Addr().IsLoopback() || endpoint.Addr().Is4In6() || endpoint.Addr().Zone() != "" || endpoint.Port() == 0 {
		return nil, errors.New("financial engine address must be a literal loopback IP with a nonzero port")
	}
	connection, err := grpc.NewClient(
		endpoint.String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxMessageBytes),
			grpc.MaxCallSendMsgSize(maxMessageBytes),
		),
	)
	if err != nil {
		return nil, errors.New("create financial engine connection failed")
	}
	return &Client{connection: connection, client: financialenginepb.NewFinancialEngineClient(connection)}, nil
}

func (c *Client) Close() error {
	return c.connection.Close()
}

func (c *Client) CategorizeTransactions(
	ctx context.Context,
	request *financialenginepb.CategorizeTransactionsRequest,
) (*financialenginepb.CategorizeTransactionsResponse, error) {
	return c.client.CategorizeTransactions(ctx, request)
}

func (c *Client) DetectRecurring(
	ctx context.Context,
	request *financialenginepb.DetectRecurringRequest,
) (*financialenginepb.DetectRecurringResponse, error) {
	return c.client.DetectRecurring(ctx, request)
}

func (c *Client) BuildProjection(
	ctx context.Context,
	request *financialenginepb.BuildProjectionRequest,
) (*financialenginepb.BuildProjectionResponse, error) {
	return c.client.BuildProjection(ctx, request)
}

func (c *Client) CalculateAvailableToSpend(
	ctx context.Context,
	request *financialenginepb.CalculateAvailableToSpendRequest,
) (*financialenginepb.CalculateAvailableToSpendResponse, error) {
	return c.client.CalculateAvailableToSpend(ctx, request)
}

func (c *Client) CalculateCashFlow(
	ctx context.Context,
	request *financialenginepb.CalculateCashFlowRequest,
) (*financialenginepb.CalculateCashFlowResponse, error) {
	return c.client.CalculateCashFlow(ctx, request)
}

func (c *Client) CalculateAnalyticsBreakdown(
	ctx context.Context,
	request *financialenginepb.CalculateAnalyticsBreakdownRequest,
) (*financialenginepb.CalculateAnalyticsBreakdownResponse, error) {
	return c.client.CalculateAnalyticsBreakdown(ctx, request)
}

func (c *Client) CalculateNetWorth(
	ctx context.Context,
	request *financialenginepb.CalculateNetWorthRequest,
) (*financialenginepb.CalculateNetWorthResponse, error) {
	return c.client.CalculateNetWorth(ctx, request)
}

func (c *Client) CalculateSubscriptionSummary(
	ctx context.Context,
	request *financialenginepb.CalculateSubscriptionSummaryRequest,
) (*financialenginepb.CalculateSubscriptionSummaryResponse, error) {
	return c.client.CalculateSubscriptionSummary(ctx, request)
}

func (c *Client) CalculatePlanningSummary(
	ctx context.Context,
	request *financialenginepb.CalculatePlanningSummaryRequest,
) (*financialenginepb.CalculatePlanningSummaryResponse, error) {
	return c.client.CalculatePlanningSummary(ctx, request)
}
