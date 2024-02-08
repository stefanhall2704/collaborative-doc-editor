// script.js
$(document).ready(function () {
  // When the user clicks on the button, open the modal
  $("#createDocument").click(function () {
    $("#createDocModal").css("display", "block");
  });

  // When the user clicks on <span> (x), close the modal
  $(".close").click(function () {
    $("#createDocModal").css("display", "none");
  });

  // When the user clicks anywhere outside of the modal, close it
  $(window).click(function (event) {
    if ($(event.target).is("#createDocModal")) {
      $("#createDocModal").css("display", "none");
    }
  });

  // Handle form submission
  $("#createDocForm").submit(function (e) {
    e.preventDefault(); // Prevent default form submission

    $.ajax({
      type: "POST",
      url: "/documents/create",
      data: $(this).serialize(), // Serializes the form's elements.
      success: function (data) {
        alert(data); // Show response from the server
        $("#createDocModal").css("display", "none"); // Close the modal
        // Optionally, refresh the page or update the UI
      },
    });
  });
});
