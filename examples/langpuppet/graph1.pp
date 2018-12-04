class mgmt_first_handover {}
class mgmt_second_handover {}

include mgmt_first_handover, mgmt_second_handover

Class["mgmt_first_handover"]
->
notify { "second message": }
->
Class["mgmt_second_handover"]
