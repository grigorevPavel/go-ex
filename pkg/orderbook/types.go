package orderbook

type Tick int64

type Price int64

type Qty int64

type ID uint64

type OrderID ID

type TradeID ID

type Side uint8

const (
	SideInvalid Side = iota // 0 by default, means not initialized order side
	Bid
	Ask
)

type OrderType uint8

const (
	OrderTypeInvalid OrderType = iota // 0 by default, means not initialized order type
	Limit
	Market
)

type Status uint8

const (
	StatusInvalid Status = iota // 0 by default, means not initialized order status
	Created
	PartiallyFilled
	Filled
	Cancelled
)

type TIF uint8

const (
	TIFInvalid TIF = iota // 0 by default, means not initialized order TIF
	GTC                   // Good Till Canceled
	IOC                   // Immediate Or Cancel
	FOK                   // Fill Or Kill
)

type Order struct {
	ID        OrderID
	Side      Side
	Type      OrderType
	TIF       TIF
	Price     Price
	Qty       Qty
	Remaining Qty
	Status    Status
}

type Trade struct {
	ID        TradeID
	MakerID   OrderID
	TakerID   OrderID
	Price     Price
	Qty       Qty
	MakerSide Side
}

type OrderNode struct {
	Order
	prev *OrderNode
	next *OrderNode
}

type OrderBookLevel struct {
	head     *OrderNode      // link to first order
	tail     *OrderNode      // link to last order
	totalQty Qty             // total quantity of orders at this level
	prev     *OrderBookLevel // link to previous level
	next     *OrderBookLevel // link to next level
}

type OrderBookLevels map[Price]*OrderBookLevel

type OrderBook struct {
	Bids         OrderBookLevels
	Asks         OrderBookLevels
	BestBidLevel *OrderBookLevel
	BestAskLevel *OrderBookLevel
	TotalBidQty  Qty
	TotalAskQty  Qty
	Registry     OrderRegistry
}

type OrderRegistry map[OrderID]*OrderNode

type PlaceOrderParams struct {
	ID    OrderID
	Side  Side
	Type  OrderType
	TIF   TIF
	Price Price
	Qty   Qty
}

type PlaceOrderResult struct {
	Trades       []Trade
	RemainingQty Qty
	Status       Status
	Resting      bool // true if the order is still resting in the order book
}

type MatchResult struct {
	Trades []Trade
}

type CancelOrderParams struct {
	ID OrderID
}

type CancelOrderResult struct {
	CancelledQty Qty  // remaining quantity of the order that was cancelled
	Success      bool // true if the order was cancelled successfully
}

type ReplaceOrderParams struct {
	ID    OrderID
	Qty   Qty
	Price Price
}

type ReplaceOrderResult struct {
	PlaceOrderResult // embed PlaceOrderResult to reuse the fields
}
