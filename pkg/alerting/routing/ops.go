package routing

import (
	"fmt"

	alertingv1alpha "github.com/rancher/opni/plugins/alerting/pkg/apis/common"
	"golang.org/x/exp/slices"
)

// CreateRoutingNodeForCondition Takes a list of endpoint details and attaches them to a route
// that fires on conditionId
//
// Modifies the internalRouting struct in place
func (r *RoutingTree) CreateRoutingNodeForCondition(
	conditionId string,
	endpoints *alertingv1alpha.FullAttachedEndpoints,
	internalRouting *OpniInternalRouting,
) error {
	if endpoints.GetItems() == nil || len(endpoints.GetItems()) == 0 {
		return fmt.Errorf("no endpoints provided")
	}
	route := NewRouteBase(conditionId)
	err := UpdateRouteWithGeneralRequestInfo(route, endpoints)
	if err != nil {
		return err
	}
	recv := NewReceiverBase(conditionId)
	for _, endpoint := range endpoints.GetItems() {
		endpointId, alertEndpoint, details := endpoint.EndpointId, endpoint.GetAlertEndpoint(), endpoints.Details
		pos, eType, err := recv.AddEndpoint(alertEndpoint, details)
		if err != nil {
			return err
		}
		err = internalRouting.Add(conditionId, endpointId, OpniRoutingMetadata{
			EndpointType: eType,
			Position:     &pos,
		})
		if err != nil {
			return err
		}
	}
	r.AppendRoute(route)
	r.AppendReceiver(recv)
	return nil
}

func (r *RoutingTree) UpdateRoutingNodeForCondition(
	conditionId string,
	endpoints *alertingv1alpha.FullAttachedEndpoints,
	internalRouting *OpniInternalRouting,
) error {
	err := internalRouting.RemoveCondition(conditionId)
	if err != nil {
		return err
	}
	err = r.DeleteReceiver(conditionId)
	if err != nil {
		return err
	}
	route := NewRouteBase(conditionId)
	err = UpdateRouteWithGeneralRequestInfo(route, endpoints)
	if err != nil {
		return err
	}
	recv := NewReceiverBase(conditionId)
	for _, endpoint := range endpoints.GetItems() {
		endpointId, alertEndpoint, details := endpoint.EndpointId, endpoint.GetAlertEndpoint(), endpoints.Details
		pos, eType, err := recv.AddEndpoint(alertEndpoint, details)
		if err != nil {
			return err
		}
		err = internalRouting.Add(conditionId, endpointId, OpniRoutingMetadata{
			EndpointType: eType,
			Position:     &pos,
		})
		if err != nil {
			return err
		}
	}
	r.AppendRoute(route)
	r.AppendReceiver(recv)
	return nil
}

func (r *RoutingTree) DeleteRoutingNodeForCondition(
	conditionId string,
	internalRouting *OpniInternalRouting,
) error {
	err := internalRouting.RemoveCondition(conditionId)
	if err != nil {
		return err
	}
	err = r.DeleteReceiver(conditionId)
	if err != nil {
		return err
	}
	return r.DeleteRoute(conditionId)
}

type TraversalOp struct {
	conditionId  string
	endpointType string
	position     int
	details      *alertingv1alpha.EndpointImplementation
}

// UpdateIndividualEndpointNode
//
// req contains the new updated details
func (r *RoutingTree) UpdateIndividualEndpointNode(
	req *alertingv1alpha.FullAttachedEndpoint,
	internalRouting *OpniInternalRouting,
) error {
	//FIXME: temporary solution
	toTraverse := []TraversalOp{}
	newEndpointTypeFunc := func() string {
		if s := req.GetAlertEndpoint().GetSlack(); s != nil {
			return SlackEndpointInternalId
		}
		if e := req.GetAlertEndpoint().GetEmail(); e != nil {
			return EmailEndpointInternalId
		}
		return "unknown"
	}
	newEndpointType := newEndpointTypeFunc()
	if newEndpointType == "unknown" {
		return fmt.Errorf("unknown endpoint type : if deleting, delete requests should be forwarded to DeleteIndividualEndpointNode")
	}
	for conditionId, routingMap := range internalRouting.Content {
		for endpointId, metadata := range routingMap {
			if endpointId == req.EndpointId {
				details, err := r.ExtractImplementationDetails(conditionId, metadata.EndpointType, *metadata.Position)
				if err != nil {
					return err
				}

				toTraverse = append(toTraverse, TraversalOp{
					conditionId:  conditionId,
					endpointType: metadata.EndpointType,
					position:     *metadata.Position,
					details:      details,
				})
			}
		}
	}
	if len(toTraverse) == 0 {
		return fmt.Errorf("no endpoint found with id %s", req.EndpointId)
	}
	// update in place
	if newEndpointType == toTraverse[0].endpointType {
		for _, toTraverseItem := range toTraverse {
			recvPos, err := r.FindReceivers(toTraverseItem.conditionId)
			if err != nil {
				return err
			}
			switch toTraverseItem.endpointType {
			case SlackEndpointInternalId:
				slackCfg, err := NewSlackReceiverNode(req.GetAlertEndpoint().GetSlack())
				if err != nil {
					return err
				}
				slackCfg, err = WithSlackImplementation(slackCfg, toTraverseItem.details)
				if err != nil {
					return err
				}
				r.Receivers[recvPos].SlackConfigs[toTraverseItem.position] = slackCfg
			case EmailEndpointInternalId:
				emailCfg, err := NewEmailReceiverNode(req.GetAlertEndpoint().GetEmail())
				if err != nil {
					return err
				}
				emailCfg, err = WithEmailImplementation(emailCfg, toTraverseItem.details)
				if err != nil {
					return err
				}
				r.Receivers[recvPos].EmailConfigs[toTraverseItem.position] = emailCfg
			}
		}
	} else {
		// delete re-add, and re-index all  routes with the same type with pos > oldPos
		for _, toTraverseItem := range toTraverse {
			// delete & re-index existing internal routing
			err := internalRouting.RemoveEndpoint(toTraverseItem.conditionId, req.EndpointId)
			if err != nil {
				return err
			}
			for _, metadata := range internalRouting.Content[toTraverseItem.conditionId] {
				if metadata.EndpointType == toTraverseItem.endpointType && *metadata.Position > toTraverseItem.position {
					*metadata.Position -= 1
				}
			}
			// add with correct config while updating internal routing
			recvPos, err := r.FindReceivers(toTraverseItem.conditionId)
			if err != nil {
				return err
			}
			switch toTraverseItem.endpointType {
			case SlackEndpointInternalId:
				r.Receivers[recvPos].SlackConfigs = slices.Delete(
					r.Receivers[recvPos].SlackConfigs,
					toTraverseItem.position,
					toTraverseItem.position+1)
			case EmailEndpointInternalId:
				r.Receivers[recvPos].EmailConfigs = slices.Delete(
					r.Receivers[recvPos].EmailConfigs,
					toTraverseItem.position,
					toTraverseItem.position+1)
			}

			newPos, newType, err := r.Receivers[recvPos].AddEndpoint(req.GetAlertEndpoint(), toTraverseItem.details)
			if err != nil {
				return err
			}
			err = internalRouting.Add(toTraverseItem.conditionId, req.EndpointId, OpniRoutingMetadata{
				EndpointType: newType,
				Position:     &newPos,
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// DeleteIndividualEndpointNode
//
// Returns a list of deleted conditions (if deleting this endpoint means they no longer have)
func (r *RoutingTree) DeleteIndividualEndpointNode(
	notificationId string,
	internalRouting *OpniInternalRouting,
) ([]string, error) {
	//FIXME: temporary solution
	toTraverse := []TraversalOp{}
	for conditionId, routingMap := range internalRouting.Content {
		for endpointId, metadata := range routingMap {
			if endpointId == notificationId {
				toTraverse = append(toTraverse, TraversalOp{
					conditionId:  conditionId,
					endpointType: metadata.EndpointType,
					position:     *metadata.Position,
				})
			}
		}
	}
	for _, toTraverseItem := range toTraverse {
		err := internalRouting.RemoveEndpoint(toTraverseItem.conditionId, notificationId)
		if err != nil {
			return nil, err
		}
		for _, metadata := range internalRouting.Content[toTraverseItem.conditionId] {
			if metadata.EndpointType == toTraverseItem.endpointType && *metadata.Position > toTraverseItem.position {
				*metadata.Position -= 1
			}
		}
		// add with correct config while updating internal routing
		recvPos, err := r.FindReceivers(toTraverseItem.conditionId)
		if err != nil {
			return nil, err
		}
		switch toTraverseItem.endpointType {
		case SlackEndpointInternalId:
			r.Receivers[recvPos].SlackConfigs = slices.Delete(
				r.Receivers[recvPos].SlackConfigs,
				toTraverseItem.position,
				toTraverseItem.position+1)
		case EmailEndpointInternalId:
			r.Receivers[recvPos].EmailConfigs = slices.Delete(
				r.Receivers[recvPos].EmailConfigs,
				toTraverseItem.position,
				toTraverseItem.position+1)
		}
	}
	// clean up
	toDelete := []string{}
	for _, receiver := range r.Receivers {
		if receiver.IsEmpty() {
			// matches the conditionId
			toDelete = append(toDelete, receiver.Name)
		}
	}
	for _, conditionId := range toDelete {
		err := r.DeleteReceiver(conditionId)
		if err != nil {
			return nil, err
		}
		err = r.DeleteRoute(conditionId)
		if err != nil {
			return nil, err
		}
	}
	return toDelete, nil
}
