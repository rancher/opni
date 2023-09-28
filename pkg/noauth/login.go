package noauth

import (
	"html/template"
	"net/http"
	"sort"

	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *Server) renderLoginPage(w http.ResponseWriter, r *http.Request) {
	rbs, err := s.mgmtApiClient.ListRoleBindings(r.Context(), &emptypb.Empty{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	knownusers := map[string]struct{}{}
	for _, rb := range rbs.Items {
		for _, subject := range rb.Subjects {
			knownusers[subject] = struct{}{}
		}
	}
	allUsersSorted := make([]string, 0, len(knownusers))
	for u := range knownusers {
		allUsersSorted = append(allUsersSorted, u)
	}
	sort.Strings(allUsersSorted)
	data := templateData{
		Users: allUsersSorted,
	}
	tmpl := template.New("index.html")
	siteTemplate, err := tmpl.ParseFS(webFS, "web/templates/index.html")
	if err != nil {
		return
	}
	siteTemplate.Execute(w, data)
}
