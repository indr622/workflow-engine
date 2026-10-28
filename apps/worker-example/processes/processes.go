// Package processes menyematkan model BPMN demo agar worker bisa men-deploy-nya sendiri.
package processes

import _ "embed"

// OrderProcess adalah order-process.bpmn: validate-order → charge-payment → ship-order.
//
//go:embed order-process.bpmn
var OrderProcess []byte

// OrderProcessID adalah bpmnProcessId model demo.
const OrderProcessID = "order-process"
