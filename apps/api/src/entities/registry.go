package entities

type Kind string

type Def struct {
	Kind  Kind
	Table string
}

const (
	User                Kind = "USER"
	BankConnection      Kind = "BANK_CONNECTION"
	Account             Kind = "ACCOUNT"
	Transaction         Kind = "TRANSACTION"
	ExpenseSplit        Kind = "EXPENSE_SPLIT"
	Category            Kind = "CATEGORY"
	Tag                 Kind = "TAG"
	Space               Kind = "SPACE"
	Bill                Kind = "BILL"
	Subscription        Kind = "SUBSCRIPTION"
	RecurringIncome     Kind = "RECURRING_INCOME"
	Budget              Kind = "BUDGET"
	Goal                Kind = "GOAL"
	Rule                Kind = "RULE"
	Household           Kind = "HOUSEHOLD"
	InboxItem           Kind = "INBOX_ITEM"
	Notification        Kind = "NOTIFICATION"
	ManualAsset         Kind = "MANUAL_ASSET"
	ManualLiability     Kind = "MANUAL_LIABILITY"
	Loan                Kind = "LOAN"
	NetWorthSnapshot    Kind = "NET_WORTH_SNAPSHOT"
	FinancialProjection Kind = "FINANCIAL_PROJECTION"
	AnalyticsSnapshot   Kind = "ANALYTICS_SNAPSHOT"
	CashFlowSnapshot    Kind = "CASH_FLOW_SNAPSHOT"
	TimelineSnapshot    Kind = "TIMELINE_SNAPSHOT"
)

var Registry = map[Kind]Def{
	User:                {Kind: User, Table: "users"},
	BankConnection:      {Kind: BankConnection, Table: "bank_connections"},
	Account:             {Kind: Account, Table: "accounts"},
	Transaction:         {Kind: Transaction, Table: "transactions"},
	ExpenseSplit:        {Kind: ExpenseSplit, Table: "expense_splits"},
	Category:            {Kind: Category, Table: "categories"},
	Tag:                 {Kind: Tag, Table: "tags"},
	Space:               {Kind: Space, Table: "spaces"},
	Bill:                {Kind: Bill, Table: "bills"},
	Subscription:        {Kind: Subscription, Table: "subscriptions"},
	RecurringIncome:     {Kind: RecurringIncome, Table: "recurring_income_sources"},
	Budget:              {Kind: Budget, Table: "budgets"},
	Goal:                {Kind: Goal, Table: "goals"},
	Rule:                {Kind: Rule, Table: "rules"},
	Household:           {Kind: Household, Table: "households"},
	InboxItem:           {Kind: InboxItem, Table: "inbox_items"},
	Notification:        {Kind: Notification, Table: "notifications"},
	ManualAsset:         {Kind: ManualAsset, Table: "manual_assets"},
	ManualLiability:     {Kind: ManualLiability, Table: "manual_liabilities"},
	Loan:                {Kind: Loan, Table: "loans"},
	NetWorthSnapshot:    {Kind: NetWorthSnapshot, Table: "net_worth_snapshots"},
	FinancialProjection: {Kind: FinancialProjection, Table: "financial_projections"},
	AnalyticsSnapshot:   {Kind: AnalyticsSnapshot, Table: "analytics_snapshots"},
	CashFlowSnapshot:    {Kind: CashFlowSnapshot, Table: "cash_flow_snapshots"},
	TimelineSnapshot:    {Kind: TimelineSnapshot, Table: "timeline_snapshots"},
}
