package orderbook

import "errors"

// --------------------- Public Methods ---------------------

func NewOrderBook() *OrderBook {
	return &OrderBook{
		Bids:         make(OrderBookLevels),
		Asks:         make(OrderBookLevels),
		BestBidLevel: nil,
		BestAskLevel: nil,
		TotalBidQty:  0,
		TotalAskQty:  0,
		Registry:     make(OrderRegistry),
		ExpiryHeap:   NewExpiryHeap(),
	}
}

func (ob *OrderBook) PlaceOrder(params PlaceOrderParams) (PlaceOrderResult, error) {
	// validate order params
	if _, exists := ob.Registry[params.ID]; exists {
		return PlaceOrderResult{}, errors.New("order ID is already in registry")
	}

	matchParams := MatchParams{
		OrderID:  params.ID,
		Side:     params.Side,
		Type:     params.Type,
		Price:    params.Price,
		Qty:      params.Qty,
		TIF:      params.TIF,
		PostOnly: params.PostOnly,
	}

	// match order with existing orders in book
	matchResult, err := ob.match(matchParams)
	if err != nil {
		return PlaceOrderResult{}, err
	}

	// add order to book if order need to be resting
	if matchResult.Resting {
		newOrder := Order{
			ID:        params.ID,
			Side:      params.Side,
			Type:      params.Type,
			Price:     params.Price,
			Qty:       params.Qty,
			TIF:       params.TIF,
			Remaining: matchResult.RemainingQty,
			Status:    matchResult.Status,
			Expiry:    params.Expiry,
		}
		ob.addOrder(&OrderNode{
			Order: newOrder,
		})
	}

	return PlaceOrderResult{
		Trades:       matchResult.Trades,
		RemainingQty: matchResult.RemainingQty,
		Status:       matchResult.Status,
		Resting:      matchResult.Resting,
	}, nil
}

func (ob *OrderBook) CancelOrder(params CancelOrderParams) (CancelOrderResult, error) {
	order, exists := ob.Registry[params.ID]
	if !exists {
		return CancelOrderResult{}, errors.New("order not found")
	}

	level, err := ob.getLevel(order.Side, order.Price)
	if err != nil {
		panic("INVALID_STATE: " + err.Error())
	}

	order.Order.Status = Cancelled
	ob.unlinkOrder(level, order)

	return CancelOrderResult{
		CancelledQty: order.Remaining,
	}, nil
}

func (ob *OrderBook) ReplaceOrder(params ReplaceOrderParams) (ReplaceOrderResult, error) {
	order, exists := ob.Registry[params.ID]
	if !exists {
		return ReplaceOrderResult{}, errors.New("order not found")
	}

	level, err := ob.getLevel(order.Side, order.Price)
	if err != nil {
		panic("INVALID_STATE: " + err.Error())
	}

	// try to match it with orderbook
	matchResult, err := ob.match(MatchParams{
		OrderID:  params.ID,
		Side:     order.Side,
		Type:     order.Type,
		TIF:      order.TIF,
		Price:    params.Price,
		Qty:      params.Qty,
		PostOnly: params.PostOnly,
	})

	// do not modify odrerbook if new order can not be matched
	if err != nil {
		return ReplaceOrderResult{}, err
	}

	// remove old order from orderbook first
	ob.unlinkOrder(level, order)

	// add new order to orderbook if it is resting
	if matchResult.Resting {
		ob.addOrder(&OrderNode{
			Order: Order{
				ID:        params.ID,
				Side:      order.Side,
				Type:      order.Type,
				TIF:       order.TIF,
				Price:     params.Price,
				Qty:       params.Qty,
				Remaining: matchResult.RemainingQty,
				Status:    matchResult.Status,
				Expiry:    params.Expiry,
			},
		})
	}

	return ReplaceOrderResult{
		PlaceOrderResult: PlaceOrderResult{
			Trades:       matchResult.Trades,
			RemainingQty: matchResult.RemainingQty,
			Status:       matchResult.Status,
			Resting:      matchResult.Resting,
		},
	}, nil
}

func (ob *OrderBook) bestLevel(side Side) (*OrderBookLevel, error) {
	if side == SideInvalid {
		return nil, errors.New("side is invalid")
	}
	if side == Bid {
		return ob.BestBidLevel, nil
	} else {
		return ob.BestAskLevel, nil
	}
}

func (ob *OrderBook) getLevel(side Side, price Price) (*OrderBookLevel, error) {
	if side == SideInvalid {
		return nil, errors.New("side is invalid")
	}

	var levels OrderBookLevels
	if side == Bid {
		levels = ob.Bids
	} else {
		levels = ob.Asks
	}

	level, exists := levels[price]
	if !exists {
		return nil, errors.New("level not found")
	}

	return level, nil
}

func (ob *OrderBook) unlinkLevel(level *OrderBookLevel) {
	if level.ordersCnt > 0 {
		panic("INVALID_STATE: level is not empty")
	}

	if level.totalQty > 0 {
		panic("INVALID_STATE: level total qty is not zero")
	}

	// unlink level from book
	if level.prev != nil {
		level.prev.next = level.next
	}
	if level.next != nil {
		level.next.prev = level.prev
	}

	// update best stats
	if ob.BestBidLevel == level {
		ob.BestBidLevel = level.next
	}
	if ob.BestAskLevel == level {
		ob.BestAskLevel = level.next
	}

	// clear levels map
	if level.side == Bid {
		delete(ob.Bids, level.price)
	} else {
		delete(ob.Asks, level.price)
	}

}

func (ob *OrderBook) unlinkOrder(level *OrderBookLevel, order *OrderNode) {
	// unlink order from level
	if order.prev != nil {
		order.prev.next = order.next
	}
	if order.next != nil {
		order.next.prev = order.prev
	}

	// update level head and tail if needed
	if level.head == order {
		level.head = order.next
	}
	if level.tail == order {
		level.tail = order.prev
	}

	// update level stats
	level.totalQty -= order.Remaining
	level.ordersCnt--

	// update book stats
	if level.side == Bid {
		ob.TotalBidQty -= order.Remaining
	} else {
		ob.TotalAskQty -= order.Remaining
	}

	// unregister order
	delete(ob.Registry, order.ID)

	// if level is empty, remove it
	if level.ordersCnt == 0 {
		ob.unlinkLevel(level) // checked ordersCnt == 0
	}

	// untrack order expiration
	ob.untrackOrderExpiration(order.Order)
}

func (ob *OrderBook) linkLevel(level *OrderBookLevel) {
	if level.side == SideInvalid {
		panic("INVALID_STATE: side is invalid")
	}

	var bookSide OrderBookLevels
	var bestLevel *OrderBookLevel
	var cmpLess func(a, b *OrderBookLevel) bool
	if level.side == Bid {
		bookSide = ob.Bids
		bestLevel = ob.BestBidLevel
		cmpLess = func(a, b *OrderBookLevel) bool {
			return a.price > b.price
		}
	} else {
		bookSide = ob.Asks
		bestLevel = ob.BestAskLevel
		cmpLess = func(a, b *OrderBookLevel) bool {
			return a.price < b.price
		}
	}

	// check if level price is already in book
	if _, exists := bookSide[level.price]; exists {
		panic("INVALID_STATE: level price is already in book")
	}

	// add level to book
	bookSide[level.price] = level
	level.prev = nil
	level.next = nil

	// link level to book (go through the book levels and find the correct position)
	var prevLevel *OrderBookLevel
	currentLevel := bestLevel
	for currentLevel != nil && cmpLess(currentLevel, level) {
		prevLevel = currentLevel
		currentLevel = currentLevel.next
	}

	level.prev = prevLevel
	level.next = currentLevel

	if prevLevel != nil {
		prevLevel.next = level
	} else {
		// change best level pointer
		// if new level is inserted as the new best level
		if level.side == Bid {
			ob.BestBidLevel = level
		} else {
			ob.BestAskLevel = level
		}
	}
	if currentLevel != nil {
		currentLevel.prev = level
	}
}

func (ob *OrderBook) linkOrder(level *OrderBookLevel, order *OrderNode) {
	// verify order.next & order.prev are nil
	order.next = nil
	order.prev = nil

	// append order to level
	if level.head == nil {
		level.head = order
		level.tail = order
	} else {
		level.tail.next = order
		order.prev = level.tail
		level.tail = order
	}

	// update level stats
	level.totalQty += order.Remaining
	level.ordersCnt++

	// update book stats
	if level.side == Bid {
		ob.TotalBidQty += order.Remaining
	} else {
		ob.TotalAskQty += order.Remaining
	}

	// register order with ID duplicate check
	if _, exists := ob.Registry[order.ID]; exists {
		panic("INVALID_STATE: order ID is already in registry")
	}
	ob.Registry[order.ID] = order

	// track order expiration
	ob.trackOrderExpiration(order.Order)
}

func (ob *OrderBook) addOrder(order *OrderNode) {
	var bookSide OrderBookLevels
	switch order.Side {
	case SideInvalid:
		panic("INVALID_STATE: side is invalid")
	case Bid:
		bookSide = ob.Bids
	default:
		bookSide = ob.Asks
	}

	// check if level price is already in book
	level, exists := bookSide[order.Price]
	if !exists {
		level = &OrderBookLevel{
			side:  order.Side,
			price: order.Price,
		}
		ob.linkLevel(level)
	}

	ob.linkOrder(level, order)
}

func (ob *OrderBook) trackOrderExpiration(order Order) {
	if order.Expiry == NoExpiryTimestamp {
		return
	}

	node := &ExpiryHeapNode{
		OrderID: order.ID,
		Expiry:  order.Expiry,
	}
	ob.ExpiryHeap.Push(node)
}

func (ob *OrderBook) untrackOrderExpiration(order Order) {
	if order.Expiry == NoExpiryTimestamp {
		return
	}
	if _, err := ob.ExpiryHeap.Remove(order.ID); err != nil {
		panic("INVALID_STATE: expiry heap out of sync: " + err.Error())
	}
}

func (ob *OrderBook) getSideTotalQty(side Side) Qty {
	if side == Bid {
		return ob.TotalBidQty
	} else {
		return ob.TotalAskQty
	}
}

// --------------------- Internal Methods ---------------------

func (ob *OrderBook) match(params MatchParams) (MatchResult, error) {
	if err := validateMatchParams(params); err != nil {
		return MatchResult{}, err
	}

	oppositeSide := getOppositeSide(params.Side)
	remainingQty := params.Qty

	trades := make([]Trade, 0)
	status := StatusInvalid
	resting := false

	if params.Type == Market {
		// check TIF
		if params.TIF == FOK {
			if remainingQty > ob.getSideTotalQty(oppositeSide) {
				// order can not be filled => reject FOK
				return MatchResult{
					Trades:       make([]Trade, 0),
					RemainingQty: params.Qty, // return the full order qty
					Status:       StatusInvalid,
					Resting:      false,
				}, errors.New("FOK order can not be filled")
			}
		}

		for remainingQty > 0 {
			bestLevel, err := ob.bestLevel(oppositeSide)
			if err != nil {
				return MatchResult{}, err
			}
			if bestLevel == nil {
				// consumed all available liquidity
				if remainingQty == params.Qty {
					status = Cancelled // no liquidity found
				} else {
					status = PartiallyFilled
				}

				return MatchResult{
					Trades:       trades,
					RemainingQty: remainingQty,
					Status:       status,
					Resting:      resting,
				}, nil
			}

			levelTrades := make([]Trade, 0, bestLevel.ordersCnt)
			headOrder := bestLevel.head
			if headOrder == nil {
				// best level must not be empty
				panic("INVALID_STATE: best level is empty")
			}

			for headOrder != nil {
				nextOrder := headOrder.next // copy next order link (headOrder can be removed in fillOrder)

				var trade Trade
				remainingQty, trade, err = fillOrder(ob, bestLevel, headOrder, params.OrderID, remainingQty)
				if err != nil {
					panic("INVALID_STATE: " + err.Error())
				}
				levelTrades = append(levelTrades, trade)

				if remainingQty == 0 {
					break
				}

				headOrder = nextOrder
			}
			trades = append(trades, levelTrades...) // add level trades to global trades
		}

		// remaining qty is zero => order is fully filled
		status = Filled
		// resting is false for Market orders (always)
	} else {
		// check post only flag
		if params.PostOnly && ob.wouldCross(params) {
			return MatchResult{
				Trades:       trades,
				RemainingQty: params.Qty, // return the full order qty
				Status:       StatusInvalid,
				Resting:      false,
			}, errors.New("post only order can not be filled as market order")
		}

		if params.TIF == FOK {
			// dry-run the matching flow without orderbook modification

			// get all opposite side orders, which are crossable with current order
			level, err := ob.bestLevel(oppositeSide)
			if err != nil {
				return MatchResult{}, err
			}

			for remainingQty > 0 && level != nil {
				if !crosses(level.price, params.Price, oppositeSide) {
					// best level is not crossable => finish
					break
				}

				headOrder := level.head
				for remainingQty > 0 && headOrder != nil {
					// consume order amount
					tradeQty := min(headOrder.Remaining, remainingQty)
					remainingQty -= tradeQty

					headOrder = headOrder.next
				}

				level = level.next // move to next level
			}

			if remainingQty > 0 {
				// order can not be fully filled
				return MatchResult{
					Trades:       trades,
					RemainingQty: params.Qty, // return the full order qty
					Status:       StatusInvalid,
					Resting:      false,
				}, errors.New("FOK order can not be filled")
			}
		}

		// reset remaining qty to the full order qty
		remainingQty = params.Qty

		// repeat the matching flow, but with orderbook modification
		level, err := ob.bestLevel(oppositeSide)
		if err != nil {
			return MatchResult{}, err
		}
		for remainingQty > 0 && level != nil {
			if !crosses(level.price, params.Price, oppositeSide) {
				// best level is not crossable => finish
				break
			}

			headOrder := level.head
			for remainingQty > 0 && headOrder != nil {
				next := headOrder.next // copy next order link (headOrder can be removed in fillOrder)
				// consume order amount
				var trade Trade
				remainingQty, trade, err = fillOrder(ob, level, headOrder, params.OrderID, remainingQty)
				if err != nil {
					panic("INVALID_STATE: " + err.Error())
				}
				trades = append(trades, trade)
				headOrder = next
			}

			level = level.next // move to next level
		}

		if remainingQty == 0 {
			status = Filled
			// resting = false
		} else {
			if remainingQty < params.Qty {
				status = PartiallyFilled
			} else {
				// 0 amount filled
				// GTC => created + resting
				// IOC => cancelled + non-resting
				if params.TIF == IOC {
					status = Cancelled
				} else {
					status = Created
				}
			}
			resting = true && params.TIF == GTC
		}
	}

	return MatchResult{
		Trades:       trades,
		RemainingQty: remainingQty,
		Status:       status,
		Resting:      resting,
	}, nil
}

func (ob *OrderBook) allocTradeID() TradeID {
	ob.NextTradeID++
	return ob.NextTradeID
}

func (ob *OrderBook) wouldCross(params MatchParams) bool {
	if params.Type == Market {
		return false
	}

	opSide := getOppositeSide(params.Side)
	level, _ := ob.bestLevel(opSide)
	if level == nil {
		return false
	}
	return crosses(level.price, params.Price, opSide)
}

// ---------------- Utils ----------------

func validateMatchParams(params MatchParams) error {
	if params.Side == SideInvalid {
		return errors.New("side is invalid")
	}
	if params.Type == OrderTypeInvalid {
		return errors.New("type is invalid")
	}
	if params.TIF == TIFInvalid {
		return errors.New("tif is invalid")
	}
	if params.Type == Market && params.Price != 0 {
		return errors.New("price is invalid")
	}
	if params.Type == Limit && params.Price <= 0 {
		return errors.New("price is invalid")
	}
	if params.Qty <= 0 {
		return errors.New("qty is invalid")
	}

	// validate TIF for order type
	if params.Type == Market {
		if params.TIF == GTC {
			return errors.New("GTC is not allowed for market orders")
		}

		if params.PostOnly {
			return errors.New("post only is not allowed for market orders")
		}
	}

	if params.PostOnly && params.Type == Limit && params.TIF != GTC {
		return errors.New("post-only only allowed with GTC")
	}

	return nil
}

func fillOrder(ob *OrderBook, level *OrderBookLevel, order *OrderNode, currentID OrderID, remainingQty Qty) (Qty, Trade, error) {
	tradeQty := min(order.Remaining, remainingQty)

	if tradeQty == 0 {
		return remainingQty, Trade{}, errors.New("no trade qty available")
	}

	// update order stats
	order.Remaining -= tradeQty

	// update level stats
	level.totalQty -= tradeQty

	// update book stats
	if level.side == Bid {
		ob.TotalBidQty -= tradeQty
	} else {
		ob.TotalAskQty -= tradeQty
	}

	trade := Trade{
		ID:        ob.allocTradeID(),
		MakerID:   order.ID,
		TakerID:   currentID,
		Price:     order.Price,
		Qty:       tradeQty,
		MakerSide: order.Side,
	}

	// update order status and remove it from book if it is fully filled
	if order.Remaining == 0 {
		order.Status = Filled
		ob.unlinkOrder(level, order)
	} else {
		order.Status = PartiallyFilled
	}

	return remainingQty - tradeQty, trade, nil
}

func crosses(levelPrice, orderPrice Price, oppositeSide Side) bool {
	if oppositeSide == SideInvalid {
		panic("INVALID_STATE: side is invalid")
	}
	if oppositeSide == Bid {
		return levelPrice >= orderPrice
	} else {
		return levelPrice <= orderPrice
	}
}

func getOppositeSide(side Side) Side {
	switch side {
	case SideInvalid:
		return SideInvalid
	case Bid:
		return Ask
	default:
		return Bid
	}
}
